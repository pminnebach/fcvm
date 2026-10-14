package vm

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/sirupsen/logrus"

	"github.com/pminnebach/fcvm/config"
	"github.com/pminnebach/fcvm/network"
	"github.com/pminnebach/fcvm/rootfs"
)

// testHost stands in for everything that needs root, KVM or Firecracker. VMs
// "run" as real sleep processes so liveness, signalling and PID reuse are
// exercised by the real code; host commands are recorded, never executed.
type testHost struct {
	t         *testing.T
	mu        sync.Mutex
	ran       [][]string
	launchErr error
	procs     map[string]*exec.Cmd
	launches  int
	patched   int
	cniIPs    []string // CNI guest addresses to hand out, one per launch
}

func (h *testHost) commands() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, c := range h.ran {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

func (h *testHost) ranCommand(prefix string) bool {
	for _, c := range h.commands() {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func (h *testHost) countCommands(prefix string) int {
	n := 0
	for _, c := range h.commands() {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

// crash kills the VM's process without going through fcvm, as a crash would.
func (h *testHost) crash(id string) {
	h.t.Helper()
	p := h.procs[id]
	_ = p.Process.Kill()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat("/proc/" + strconv.Itoa(p.Process.Pid)); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatal("process did not exit")
}

func newTestManager(t *testing.T) (*Manager, *testHost) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = dir
	cfg.Jailer.ChrootBaseDir = filepath.Join(dir, "jailer")
	cfg.Jailer.UID, cfg.Jailer.GID = os.Getuid(), os.Getgid() // chown to self works unprivileged
	cfg.SSHKey = filepath.Join(dir, "key")
	cfg.StopTimeoutSec = 5
	cfg.WaitTimeoutSec = 1
	for _, name := range []string{"firecracker", "jailer", "vmlinux", "rootfs.ext4"} {
		p := filepath.Join(dir, "assets", name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("template"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg.FirecrackerBin = filepath.Join(dir, "assets", "firecracker")
	cfg.JailerBin = filepath.Join(dir, "assets", "jailer")
	cfg.Kernel = filepath.Join(dir, "assets", "vmlinux")
	cfg.Rootfs = filepath.Join(dir, "assets", "rootfs.ext4")

	h := &testHost{t: t, procs: map[string]*exec.Cmd{}}
	restore := network.SetRunner(func(name string, args ...string) error {
		h.mu.Lock()
		h.ran = append(h.ran, append([]string{name}, args...))
		h.mu.Unlock()
		// "ip link show" succeeding would mean the TAP already exists.
		if name == "ip" && len(args) > 1 && args[0] == "link" && args[1] == "show" {
			return errors.New("absent")
		}
		return nil
	})
	t.Cleanup(restore)

	m := NewManager(cfg)
	quiet := logrus.New()
	quiet.SetOutput(io.Discard)
	m.log = logrus.NewEntry(quiet)
	m.ops = hostOps{
		euid:            func() int { return 0 },
		defaultIface:    func() (string, error) { return "eth0", nil },
		enableIPForward: func(string) error { return nil },
		patchRootfs: func(string, rootfs.PatchOptions) error {
			h.patched++
			return nil
		},
		waitSSH:    func(context.Context, string, string, time.Duration) error { return errors.New("no ssh in tests") },
		pushMounts: func(string, string, string) error { return nil },
		launch: func(ctx context.Context, fc firecracker.Config, _ *logrus.Entry, cni bool) (*launched, error) {
			h.launches++
			if h.launchErr != nil {
				return nil, h.launchErr
			}
			cmd := exec.Command("sleep", "60")
			if err := cmd.Start(); err != nil {
				return nil, err
			}
			go func() { _ = cmd.Wait() }() // reap, or the zombie still looks alive
			t.Cleanup(func() { _ = cmd.Process.Kill() })
			h.procs[vmIDFromConfig(fc)] = cmd
			l := &launched{
				pid:         func() (int, error) { return cmd.Process.Pid, nil },
				socketPath:  fc.SocketPath,
				setMetadata: func(context.Context, interface{}) error { return nil },
				stop:        func() error { return cmd.Process.Kill() },
				wait:        func(context.Context) error { return nil },
			}
			if cni {
				l.guestIP, l.gateway, l.mac = h.cniIPs[h.launches-1], "10.0.0.1", "06:00:0a:00:00:09"
			}
			return l, nil
		},
	}
	return m, h
}

func vmIDFromConfig(fc firecracker.Config) string {
	return fc.VMID
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

func mustStart(t *testing.T, m *Manager, id string) *State {
	t.Helper()
	st, err := m.Start(context.Background(), id)
	if err != nil {
		t.Fatalf("start %q: %v", id, err)
	}
	return st
}

func statusOf(t *testing.T, m *Manager, id string) Status {
	t.Helper()
	info, err := Inspect(m.cfg.StateDir, id)
	if err != nil {
		t.Fatalf("inspect %q: %v", id, err)
	}
	return info.Status
}
