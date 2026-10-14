package vm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pminnebach/fcvm/config"
)

// --- creating a VM (ticket 01) ---

func TestStartNewVMWritesConfigurationRecord(t *testing.T) {
	m, _ := newTestManager(t)
	m.cfg.VCPUCount = 3
	m.cfg.Env = map[string]string{"A": "1"}

	st := mustStart(t, m, "web")

	vc, err := LoadVMConfig(m.cfg.StateDir, "web")
	if err != nil {
		t.Fatal(err)
	}
	if vc.Index != st.Index || vc.Config.VCPUCount != 3 || vc.Config.Env["A"] != "1" {
		t.Fatalf("configuration record = %+v", vc)
	}
	if vc.GuestIP != st.GuestIP || vc.TapDev != st.TapDev || vc.GuestIP == "" {
		t.Fatalf("record guest %q tap %q, state guest %q tap %q", vc.GuestIP, vc.TapDev, st.GuestIP, st.TapDev)
	}
	if got := statusOf(t, m, "web"); got != StatusRunning {
		t.Fatalf("status = %s", got)
	}
}

func TestFailedCreationRemovesEverything(t *testing.T) {
	m, h := newTestManager(t)
	h.launchErr = errors.New("boom")

	if _, err := m.Start(context.Background(), "doomed"); err == nil {
		t.Fatal("expected error")
	}
	if exists(t, filepath.Join(m.cfg.StateDir, "vms", "doomed")) {
		t.Fatal("a failed creation must not leave a VM behind")
	}
	if !h.ranCommand("ip link del " + "fcvm-tap-0") {
		t.Fatalf("the TAP device was not released; ran %v", h.commands())
	}
}

// --- stop keeps the VM (ticket 02) ---

func TestStopKeepsTheVM(t *testing.T) {
	m, h := newTestManager(t)
	st := mustStart(t, m, "web")
	disk := filepath.Join(m.cfg.StateDir, "vms", "web", "rootfs.ext4")
	if err := os.WriteFile(disk, []byte("guest data"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}

	info, err := Inspect(m.cfg.StateDir, "web")
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != StatusStopped || info.StoppedAt.IsZero() {
		t.Fatalf("status %s stoppedAt %v", info.Status, info.StoppedAt)
	}
	if data, _ := os.ReadFile(disk); string(data) != "guest data" {
		t.Fatalf("disk was not preserved: %q", data)
	}
	if info.State != nil {
		t.Fatal("runtime state must be removed")
	}
	if info.Config == nil || info.Config.Index != st.Index {
		t.Fatalf("configuration lost: %+v", info.Config)
	}
	if !h.ranCommand("ip link del " + st.TapDev) {
		t.Fatalf("TAP device not released; ran %v", h.commands())
	}
	if exists(t, m.jailerTreeDir("web")) {
		t.Fatal("jailer tree must be released")
	}
	proc := h.procs["web"]
	if proc.ProcessState == nil && (&State{PID: proc.Process.Pid}).IsRunning() {
		t.Fatal("process still running")
	}
}

func TestStopIsIdempotent(t *testing.T) {
	m, _ := newTestManager(t)
	mustStart(t, m, "web")
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	first, _ := Inspect(m.cfg.StateDir, "web")
	if err := m.Stop("web"); err != nil {
		t.Fatalf("second stop: %v", err)
	}
	second, _ := Inspect(m.cfg.StateDir, "web")
	if !second.StoppedAt.Equal(first.StoppedAt) {
		t.Fatal("stopping a stopped VM must not move its stop time")
	}
}

func TestStopUnknownVMFails(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.Stop("ghost"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want not-exist error, got %v", err)
	}
}

func TestStoppedVMKeepsItsIndex(t *testing.T) {
	m, _ := newTestManager(t)
	a := mustStart(t, m, "a")
	if err := m.Stop("a"); err != nil {
		t.Fatal(err)
	}
	b := mustStart(t, m, "b")
	if b.Index == a.Index || b.GuestIP == a.GuestIP {
		t.Fatalf("new VM took the stopped VM's slot: a=%d/%s b=%d/%s", a.Index, a.GuestIP, b.Index, b.GuestIP)
	}
}

func TestHostNetworkReleasedWhenLastRunningVMStops(t *testing.T) {
	m, h := newTestManager(t)
	mustStart(t, m, "a")
	mustStart(t, m, "b")

	if err := m.Stop("a"); err != nil {
		t.Fatal(err)
	}
	if h.ranCommand("iptables -X") {
		t.Fatal("the FCVM chain must stay while another VM runs")
	}
	if err := m.Stop("b"); err != nil {
		t.Fatal(err)
	}
	if !h.ranCommand("iptables -X") {
		t.Fatalf("stopped VMs must not hold the host chain open; ran %v", h.commands())
	}
}

func TestInterruptedStopReadsAsCrashed(t *testing.T) {
	m, h := newTestManager(t)
	mustStart(t, m, "web")
	h.crash("web") // no stop record: the user never stopped it
	if got := statusOf(t, m, "web"); got != StatusCrashed {
		t.Fatalf("status = %s", got)
	}
}

// --- starting an existing VM (ticket 03) ---

func TestStartStoppedVMReusesDiskAndIdentity(t *testing.T) {
	m, h := newTestManager(t)
	first := mustStart(t, m, "web")
	disk := filepath.Join(m.cfg.StateDir, "vms", "web", "rootfs.ext4")
	if err := os.WriteFile(disk, []byte("guest data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	patched := h.patched

	second := mustStart(t, m, "web")

	if data, _ := os.ReadFile(disk); string(data) != "guest data" {
		t.Fatalf("disk was rebuilt: %q", data)
	}
	if h.patched != patched {
		t.Fatal("an existing VM's rootfs must not be patched again")
	}
	if second.Index != first.Index || second.GuestIP != first.GuestIP || second.TapDev != first.TapDev {
		t.Fatalf("identity changed: %+v -> %+v", first, second)
	}
	info, _ := Inspect(m.cfg.StateDir, "web")
	if info.Status != StatusRunning || !info.StoppedAt.IsZero() {
		t.Fatalf("status %s stoppedAt %v", info.Status, info.StoppedAt)
	}
}

func TestStartExistingUsesSavedConfiguration(t *testing.T) {
	m, h := newTestManager(t)
	m.cfg.VCPUCount = 4
	m.cfg.MemSizeMib = 1024
	m.cfg.Env = map[string]string{"K": "saved"}
	mustStart(t, m, "web")
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}

	// The configuration on this run says otherwise; the saved shape must win.
	m.cfg.VCPUCount = 1
	m.cfg.MemSizeMib = 128
	m.cfg.Env = map[string]string{"K": "new"}
	st := mustStart(t, m, "web")

	if st.Env["K"] != "saved" {
		t.Fatalf("env = %v", st.Env)
	}
	_ = h
	vc, _ := LoadVMConfig(m.cfg.StateDir, "web")
	if vc.Config.VCPUCount != 4 || vc.Config.MemSizeMib != 1024 {
		t.Fatalf("the configuration record must never change: %+v", vc.Config)
	}
}

func TestStartRunningVMFails(t *testing.T) {
	m, h := newTestManager(t)
	mustStart(t, m, "web")
	launches := h.launches
	if _, err := m.Start(context.Background(), "web"); err == nil {
		t.Fatal("starting a running VM must fail")
	}
	if h.launches != launches {
		t.Fatal("a second process was launched")
	}
	if got := statusOf(t, m, "web"); got != StatusRunning {
		t.Fatalf("status = %s", got)
	}
}

func TestStartLegacyVMFails(t *testing.T) {
	m, _ := newTestManager(t)
	if err := SaveState(m.cfg.StateDir, &State{ID: "old", TapDev: "fcvm-tap-0"}); err != nil {
		t.Fatal(err)
	}
	_, err := m.Start(context.Background(), "old")
	if err == nil || !strings.Contains(err.Error(), "cleanup") {
		t.Fatalf("want a cleanup hint, got %v", err)
	}
}

func TestFailedRestartKeepsTheDisk(t *testing.T) {
	m, h := newTestManager(t)
	mustStart(t, m, "web")
	disk := filepath.Join(m.cfg.StateDir, "vms", "web", "rootfs.ext4")
	if err := os.WriteFile(disk, []byte("guest data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}

	h.launchErr = errors.New("kvm exploded")
	if _, err := m.Start(context.Background(), "web"); err == nil {
		t.Fatal("expected error")
	}

	if data, _ := os.ReadFile(disk); string(data) != "guest data" {
		t.Fatalf("a failed restart destroyed the disk: %q", data)
	}
	if got := statusOf(t, m, "web"); got != StatusStopped {
		t.Fatalf("status = %s, want stopped", got)
	}
	if _, err := LoadVMConfig(m.cfg.StateDir, "web"); err != nil {
		t.Fatal(err)
	}
	if exists(t, m.jailerTreeDir("web")) {
		t.Fatal("jailer tree left behind")
	}
}

func TestStartExistingReusesBlockMountImage(t *testing.T) {
	m, _ := newTestManager(t)
	host := t.TempDir()
	m.cfg.Mounts = []config.MountConfig{{Host: host, Guest: "/data", Method: "block", Mode: "rw"}}
	// Seed a VM as if created earlier with a block mount whose image holds
	// guest changes the host directory does not have.
	mustStartWithoutMounts := func() {
		cfg := m.cfg
		cfg.Mounts = nil
		mm := m.withConfig(cfg)
		mustStart(t, mm, "web")
	}
	mustStartWithoutMounts()
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	// Rewrite the immutable record's mounts directly: simulates a VM created
	// with a block mount.
	path := filepath.Join(m.cfg.StateDir, "vms", "web", configFile)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	vc := &VMConfig{ID: "web", Index: 0, Config: m.cfg, TapDev: "fcvm-tap-0", TapIP: "172.16.0.1", GuestIP: "172.16.0.2", GuestSubnet: "172.16.0.0/30", JailerUID: os.Getuid(), JailerGID: os.Getgid()}
	if err := SaveVMConfig(m.cfg.StateDir, vc); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(m.cfg.StateDir, "vms", "web", "mount-0.ext4")
	if err := os.WriteFile(img, []byte("guest-side changes"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := mustStart(t, m, "web")

	if data, _ := os.ReadFile(img); string(data) != "guest-side changes" {
		t.Fatalf("block image was rebuilt from the host directory: %q", data)
	}
	if len(st.Mounts) != 1 || st.Mounts[0].Method != config.MountBlock {
		t.Fatalf("mounts = %+v", st.Mounts)
	}
}

// --- restart (ticket 04) ---

func TestRestartRunningVM(t *testing.T) {
	m, h := newTestManager(t)
	first := mustStart(t, m, "web")
	disk := filepath.Join(m.cfg.StateDir, "vms", "web", "rootfs.ext4")
	if err := os.WriteFile(disk, []byte("guest data"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := m.Restart(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if second.PID == first.PID {
		t.Fatal("restart must boot a new process")
	}
	if data, _ := os.ReadFile(disk); string(data) != "guest data" {
		t.Fatalf("disk lost: %q", data)
	}
	if h.launches != 2 || statusOf(t, m, "web") != StatusRunning {
		t.Fatalf("launches %d status %s", h.launches, statusOf(t, m, "web"))
	}
}

func TestRestartStoppedAndCrashedVM(t *testing.T) {
	m, h := newTestManager(t)
	mustStart(t, m, "a")
	if err := m.Stop("a"); err != nil {
		t.Fatal(err)
	}
	mustStart(t, m, "b")
	h.crash("b")

	for _, id := range []string{"a", "b"} {
		if _, err := m.Restart(context.Background(), id); err != nil {
			t.Fatalf("restart %s: %v", id, err)
		}
		if got := statusOf(t, m, id); got != StatusRunning {
			t.Fatalf("%s status = %s", id, got)
		}
	}
}

func TestRestartUnknownVMDoesNotCreateOne(t *testing.T) {
	m, h := newTestManager(t)
	_, err := m.Restart(context.Background(), "ghost")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want not-exist error, got %v", err)
	}
	if h.launches != 0 || exists(t, filepath.Join(m.cfg.StateDir, "vms", "ghost")) {
		t.Fatal("restart must never create a VM")
	}
}

// --- delete (ticket 05) ---

func TestDeleteStoppedVMFreesItsSlot(t *testing.T) {
	m, _ := newTestManager(t)
	a := mustStart(t, m, "a")
	if err := m.Stop("a"); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete("a", false); err != nil {
		t.Fatal(err)
	}
	if exists(t, filepath.Join(m.cfg.StateDir, "vms", "a")) {
		t.Fatal("VM directory remains")
	}
	b := mustStart(t, m, "b")
	if b.Index != a.Index {
		t.Fatalf("the freed slot was not reused: %d vs %d", b.Index, a.Index)
	}
}

func TestDeleteRunningVMNeedsForce(t *testing.T) {
	m, _ := newTestManager(t)
	mustStart(t, m, "web")

	if err := m.Delete("web", false); err == nil {
		t.Fatal("deleting a running VM without force must fail")
	}
	if got := statusOf(t, m, "web"); got != StatusRunning {
		t.Fatalf("status = %s", got)
	}
	if err := m.Delete("web", true); err != nil {
		t.Fatal(err)
	}
	if exists(t, filepath.Join(m.cfg.StateDir, "vms", "web")) {
		t.Fatal("VM directory remains")
	}
}

func TestDeleteUnknownVMFails(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.Delete("ghost", false); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want not-exist error, got %v", err)
	}
}

func TestDeleteRejectsPathTraversal(t *testing.T) {
	m, _ := newTestManager(t)
	victim := filepath.Join(m.cfg.StateDir, "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete("../victim", true); err == nil {
		t.Fatal("expected invalid id error")
	}
	if !exists(t, victim) {
		t.Fatal("delete escaped the VM directory")
	}
}

// --- crashed VM recovery (ticket 06) ---

func TestStartCrashedVMTearsDownLeftoversFirst(t *testing.T) {
	m, h := newTestManager(t)
	first := mustStart(t, m, "web")
	h.crash("web")
	before := h.countCommands("ip link del " + first.TapDev)

	second := mustStart(t, m, "web")

	if h.countCommands("ip link del "+first.TapDev) <= before {
		t.Fatal("the crash's TAP device was not torn down before booting")
	}
	if second.Index != first.Index || statusOf(t, m, "web") != StatusRunning {
		t.Fatalf("not recovered: %+v", second)
	}
}

func TestStopCrashedVMRecordsTheStop(t *testing.T) {
	m, h := newTestManager(t)
	st := mustStart(t, m, "web")
	h.crash("web")

	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, m, "web"); got != StatusStopped {
		t.Fatalf("status = %s", got)
	}
	if !h.ranCommand("ip link del " + st.TapDev) {
		t.Fatal("leftovers not torn down")
	}
}

func TestStopLegacyVMDeletesIt(t *testing.T) {
	m, _ := newTestManager(t)
	if err := SaveState(m.cfg.StateDir, &State{ID: "old", TapDev: "fcvm-tap-0"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop("old"); err != nil {
		t.Fatal(err)
	}
	if exists(t, filepath.Join(m.cfg.StateDir, "vms", "old")) {
		t.Fatal("a legacy VM cannot be restarted, so stop removes it as before")
	}
}

// --- mounts and addresses across restarts (ticket 07) ---

func TestCNIGuestAddressIsResolvedAtEachStart(t *testing.T) {
	m, h := newTestManager(t)
	m.cfg.Network.CNINetwork = "fcvm"
	h.cniIPs = []string{"10.0.0.5", "10.0.0.9"}

	first := mustStart(t, m, "web")
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	second := mustStart(t, m, "web")

	if first.GuestIP != "10.0.0.5" || second.GuestIP != "10.0.0.9" {
		t.Fatalf("CNI addresses: %s then %s", first.GuestIP, second.GuestIP)
	}
	vc, _ := LoadVMConfig(m.cfg.StateDir, "web")
	if vc.GuestIP != "" || !vc.IsCNI() {
		t.Fatalf("a CNI VM must not record an address it cannot promise: %+v", vc)
	}
}

// --- unreadable records ---

// A record that cannot be parsed is not "no such VM". Treating it as one sent
// Start down the creation path, whose failure handling deletes the directory.
func TestStartWithUnreadableRecordKeepsTheDisk(t *testing.T) {
	for _, corrupt := range []string{configFile, "state.json"} {
		t.Run(corrupt, func(t *testing.T) {
			m, _ := newTestManager(t)
			mustStart(t, m, "web")
			disk := filepath.Join(m.cfg.StateDir, "vms", "web", "rootfs.ext4")
			if err := os.WriteFile(disk, []byte("guest data"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := m.Stop("web"); err != nil {
				t.Fatal(err)
			}
			bad := filepath.Join(m.cfg.StateDir, "vms", "web", corrupt)
			if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
				t.Fatal(err)
			}

			if _, err := m.Start(context.Background(), "web"); err == nil {
				t.Fatal("expected an error")
			}
			if data, _ := os.ReadFile(disk); string(data) != "guest data" {
				t.Fatalf("an unreadable record cost the user the disk: %q", data)
			}
			if data, _ := os.ReadFile(bad); string(data) != "{not json" {
				t.Fatalf("the record was touched: %q", data)
			}
		})
	}
}

// An unreadable record means the VM's index cannot be known. Allocating
// around it would hand its index, and so its guest IP, to a new VM.
func TestNewVMIsRefusedWhileARecordIsUnreadable(t *testing.T) {
	m, _ := newTestManager(t)
	a := mustStart(t, m, "a")
	if err := m.Stop("a"); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(m.cfg.StateDir, "vms", "a", configFile)
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := m.Start(context.Background(), "b")
	if err == nil {
		t.Fatalf("a new VM must not be created while %q is unreadable (index %d)", "a", a.Index)
	}
	if !strings.Contains(err.Error(), `"a"`) {
		t.Fatalf("the error should name the unreadable VM: %v", err)
	}
	if exists(t, filepath.Join(m.cfg.StateDir, "vms", "b")) {
		t.Fatal("a failed allocation left a VM behind")
	}
}

func TestListStillShowsOtherVMsWhenOneIsUnreadable(t *testing.T) {
	m, _ := newTestManager(t)
	mustStart(t, m, "a")
	mustStart(t, m, "b")
	if err := os.WriteFile(filepath.Join(m.cfg.StateDir, "vms", "a", configFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	infos, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].ID != "b" {
		t.Fatalf("infos = %+v", infos)
	}
}
