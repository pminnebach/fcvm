package vm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/sirupsen/logrus"

	"github.com/fcvm/fcvm/assets"
	"github.com/fcvm/fcvm/config"
	"github.com/fcvm/fcvm/guest"
	"github.com/fcvm/fcvm/network"
	"github.com/fcvm/fcvm/rootfs"
)

type Manager struct {
	cfg config.Config
	log *logrus.Entry
}

func NewManager(cfg config.Config) *Manager {
	log := logrus.New()
	if cfg.Verbose {
		log.SetLevel(logrus.DebugLevel)
	}
	return &Manager{cfg: cfg, log: logrus.NewEntry(log)}
}

func (m *Manager) requireRoot() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("fcvm must run as root (jailer, networking, and NFS require it)")
	}
	return nil
}

func (m *Manager) Start(ctx context.Context, id string) (*State, error) {
	if err := m.requireRoot(); err != nil {
		return nil, err
	}
	if id == "" {
		id = "vm-" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	if _, err := LoadState(m.cfg.StateDir, id); err == nil {
		return nil, fmt.Errorf("VM %q already exists; stop it first", id)
	}
	m.removeJailerTree(id)

	if err := m.cfg.Validate(); err != nil {
		return nil, err
	}
	for _, bin := range []string{m.cfg.FirecrackerBin, m.cfg.JailerBin, m.cfg.Kernel, m.cfg.Rootfs} {
		if _, err := os.Stat(bin); err != nil {
			return nil, fmt.Errorf("missing %q: %w", bin, err)
		}
	}

	vmDir := m.cfg.VMStateDir(id)
	if err := os.MkdirAll(vmDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(m.cfg.Jailer.ChrootBaseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create jailer chroot base %q: %w", m.cfg.Jailer.ChrootBaseDir, err)
	}

	key, err := guest.LoadOrCreateKey(m.cfg.SSHKey)
	if err != nil {
		return nil, err
	}

	// prepare rootfs copy with SSH key for this VM
	index := m.nextVMIndex()
	tapIP, guestIP := network.SubnetForIndex(m.cfg.Network.TapIP, m.cfg.Network.GuestIP, index)
	tapDev := fmt.Sprintf("fcvm-tap-%s", id)
	guestMAC := network.GuestMAC(guestIP)

	rootfsCopy := filepath.Join(vmDir, "rootfs.ext4")
	if err := copyFile(m.cfg.Rootfs, rootfsCopy); err != nil {
		return nil, fmt.Errorf("copy rootfs: %w", err)
	}
	if err := assets.PatchExt4(rootfsCopy, key.PublicKey); err != nil {
		return nil, fmt.Errorf("patch rootfs: %w", err)
	}
	if err := assets.PatchNetwork(rootfsCopy, guestIP, tapIP); err != nil {
		return nil, fmt.Errorf("patch guest network: %w", err)
	}
	if err := m.chownForJailer(rootfsCopy); err != nil {
		return nil, err
	}

	if err := network.SetupTap(tapDev, tapIP, guestIP); err != nil {
		return nil, err
	}

	var mountStates []MountState
	var blockImages []string
	mountMeta := []map[string]string{}

	for i, mount := range m.cfg.Mounts {
		guestPath := mount.Guest
		if guestPath == "" {
			guestPath = "/mnt/" + filepath.Base(mount.Host)
		}
		method := mount.Method
		if method == "" || method == "auto" {
			method = "nfs"
		}
		if method == "nfs" {
			exp, err := network.SetupNFSExport(mount.Host, id+"-"+strconv.Itoa(i), mount.Mode == "ro")
			if err != nil {
				m.log.Warnf("NFS unavailable for %s: %v; falling back to block device", mount.Host, err)
				method = "block"
			} else {
				mountStates = append(mountStates, MountState{
					Host: exp.ExportPath, Guest: guestPath, Method: "nfs",
				})
				mountMeta = append(mountMeta, map[string]string{
					"host": exp.ExportPath + ":/", "guest": guestPath, "method": "nfs",
				})
				continue
			}
		}
		if method == "block" {
			img := filepath.Join(vmDir, fmt.Sprintf("mount-%d.ext4", i))
			if err := syncDirToExt4(mount.Host, img); err != nil {
				network.TeardownTap(tapDev)
				return nil, err
			}
			if err := m.chownForJailer(img); err != nil {
				network.TeardownTap(tapDev)
				return nil, err
			}
			blockImages = append(blockImages, img)
			mountStates = append(mountStates, MountState{
				Host: mount.Host, Guest: guestPath, Method: "block", Device: img,
			})
			mountMeta = append(mountMeta, map[string]string{
				"host": mount.Host, "guest": guestPath, "method": "block",
			})
		}
	}

	socketPath := "firecracker.sock"
	logPath := "firecracker.log"

	kernelArgs := "console=ttyS0 reboot=k panic=1 net.ifnames=0 biosdevname=0"
	if m.cfg.ExposeKVM {
		kernelArgs += " pci=on fcvm.kvm=1"
	}

	uid, gid, numa := m.cfg.Jailer.UID, m.cfg.Jailer.GID, 0
	cgroupVer := jailerCgroupVersion()

	drives := []models.Drive{{
		DriveID:      firecracker.String("rootfs"),
		PathOnHost:   firecracker.String(rootfsCopy),
		IsRootDevice: firecracker.Bool(true),
		IsReadOnly:   firecracker.Bool(false),
	}}
	for j, img := range blockImages {
		drives = append(drives, models.Drive{
			DriveID:      firecracker.String(fmt.Sprintf("data%d", j)),
			PathOnHost:   firecracker.String(img),
			IsRootDevice: firecracker.Bool(false),
			IsReadOnly:   firecracker.Bool(false),
		})
	}

	fcCfg := firecracker.Config{
		VMID:            id,
		SocketPath:      socketPath,
		LogPath:         logPath,
		LogLevel:        "Info",
		KernelImagePath: m.cfg.Kernel,
		KernelArgs:      kernelArgs,
		Drives:          drives,
		MachineCfg: models.MachineConfiguration{
			VcpuCount:  firecracker.Int64(m.cfg.VCPUCount),
			MemSizeMib: firecracker.Int64(m.cfg.MemSizeMib),
		},
		NetworkInterfaces: []firecracker.NetworkInterface{{
			StaticConfiguration: &firecracker.StaticNetworkConfiguration{
				MacAddress:  guestMAC,
				HostDevName: tapDev,
				IPConfiguration: &firecracker.IPConfiguration{
					IPAddr: net.IPNet{
						IP:   net.ParseIP(guestIP),
						Mask: net.CIDRMask(30, 32),
					},
					Gateway:     net.ParseIP(tapIP),
					Nameservers: []string{"8.8.8.8"},
					IfName:      "eth0",
				},
			},
			AllowMMDS: true,
		}},
		MmdsVersion: firecracker.MMDSv2,
		JailerCfg: &firecracker.JailerConfig{
			JailerBinary:  m.cfg.JailerBin,
			ExecFile:      m.cfg.FirecrackerBin,
			ID:            id,
			UID:           &uid,
			GID:           &gid,
			NumaNode:      &numa,
			CgroupVersion: cgroupVer,
			ChrootBaseDir: m.cfg.Jailer.ChrootBaseDir,
			ChrootStrategy: firecracker.NewNaiveChrootStrategy(
				filepath.Base(m.cfg.Kernel),
			),
		},
	}

	machine, err := firecracker.NewMachine(ctx, fcCfg, firecracker.WithLogger(m.log))
	if err != nil {
		network.TeardownTap(tapDev)
		m.removeJailerTree(id)
		return nil, fmt.Errorf("create machine: %w", err)
	}

	if err := machine.Start(ctx); err != nil {
		network.TeardownTap(tapDev)
		m.removeJailerTree(id)
		return nil, fmt.Errorf("start machine: %w", err)
	}

	meta := map[string]interface{}{
		"env":    m.cfg.Env,
		"mounts": mountMeta,
	}
	if err := machine.SetMetadata(ctx, meta); err != nil {
		m.log.Warnf("set MMDS metadata: %v", err)
	}

	pid, _ := machine.PID()

	chrootDir := filepath.Join(m.cfg.Jailer.ChrootBaseDir, "firecracker", id, "root")
	hostSocketPath := machine.Cfg.SocketPath
	hostLogPath := filepath.Join(chrootDir, logPath)

	state := &State{
		ID:          id,
		PID:         pid,
		SocketPath:  hostSocketPath,
		TapDev:      tapDev,
		GuestIP:     guestIP,
		GuestMAC:    guestMAC,
		SSHKey:      key.PrivateKeyPath,
		ChrootDir:   chrootDir,
		LogPath:     hostLogPath,
		StartedAt:   time.Now(),
		Mounts:      mountStates,
		BlockImages: blockImages,
		Env:         m.cfg.Env,
	}
	if err := SaveState(m.cfg.StateDir, state); err != nil {
		_ = machine.StopVMM()
		return nil, err
	}

	timeout := time.Duration(m.cfg.WaitTimeoutSec) * time.Second
	if err := guest.WaitSSH(guestIP, key.PrivateKeyPath, timeout); err != nil {
		m.log.Warnf("guest ssh not ready: %v", err)
	}

	go m.waitVM(ctx, machine, id)

	return state, nil
}

func (m *Manager) waitVM(ctx context.Context, machine *firecracker.Machine, id string) {
	_ = machine.Wait(ctx)
	m.log.Infof("VM %q exited", id)
}

func (m *Manager) Stop(id string) error {
	if err := m.requireRoot(); err != nil {
		return err
	}
	state, err := LoadState(m.cfg.StateDir, id)
	if err != nil {
		return err
	}
	if state.PID > 0 {
		proc, err := os.FindProcess(state.PID)
		if err == nil {
			_ = proc.Signal(syscall.SIGTERM)
			time.Sleep(2 * time.Second)
			_ = proc.Kill()
		}
	}
	m.teardownState(state)
	return RemoveState(m.cfg.StateDir, id)
}

func (m *Manager) Cleanup(all bool, id string) error {
	if err := m.requireRoot(); err != nil {
		return err
	}
	if all {
		states, err := ListStates(m.cfg.StateDir)
		if err != nil {
			return err
		}
		for _, s := range states {
			m.teardownState(&s)
			_ = RemoveState(m.cfg.StateDir, s.ID)
		}
		return nil
	}
	if id == "" {
		return fmt.Errorf("specify VM id or --all")
	}
	state, err := LoadState(m.cfg.StateDir, id)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		m.cleanupByID(id)
		return nil
	}
	m.teardownState(state)
	return RemoveState(m.cfg.StateDir, id)
}

func (m *Manager) cleanupByID(id string) {
	network.TeardownTap(fmt.Sprintf("fcvm-tap-%s", id))
	network.TeardownNFSExport(id)
	m.removeJailerTree(id)
	_ = RemoveState(m.cfg.StateDir, id)
}

func (m *Manager) chownForJailer(path string) error {
	if err := os.Chown(path, m.cfg.Jailer.UID, m.cfg.Jailer.GID); err != nil {
		return fmt.Errorf("chown %q for jailer uid %d: %w", path, m.cfg.Jailer.UID, err)
	}
	return nil
}

func (m *Manager) jailerTreeDir(id string) string {
	return filepath.Join(m.cfg.Jailer.ChrootBaseDir, "firecracker", id)
}

func (m *Manager) removeJailerTree(id string) {
	_ = os.RemoveAll(m.jailerTreeDir(id))
}

func (m *Manager) teardownState(state *State) {
	network.TeardownTap(state.TapDev)
	network.TeardownNFSExport(state.ID)
	if state.ChrootDir != "" {
		_ = os.RemoveAll(filepath.Dir(state.ChrootDir))
	}
	_ = os.Remove(state.SocketPath)
}

func (m *Manager) List() ([]State, error) {
	return ListStates(m.cfg.StateDir)
}

func (m *Manager) nextVMIndex() int {
	states, _ := ListStates(m.cfg.StateDir)
	return len(states)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = out.ReadFrom(in)
	return err
}

func syncDirToExt4(hostDir, img string) error {
	dir, err := os.MkdirTemp("", "fcvm-sync-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	staging := filepath.Join(dir, "staging")
	if out, err := exec.Command("cp", "-a", hostDir+"/.", staging).CombinedOutput(); err != nil {
		// cp -a might fail if hostDir empty; try mkdir + rsync
		_ = os.MkdirAll(staging, 0o755)
		if out2, err2 := exec.Command("rsync", "-a", hostDir+"/", staging+"/").CombinedOutput(); err2 != nil {
			return fmt.Errorf("sync dir: %s / %s: %w", out, out2, err2)
		}
	}
	if err := rootfs.InjectHooks(staging); err != nil {
		return err
	}
	if err := os.Remove(img); err != nil && !os.IsNotExist(err) {
		return err
	}
	if out, err := exec.Command("truncate", "-s", "512M", img).CombinedOutput(); err != nil {
		return fmt.Errorf("truncate: %s: %w", out, err)
	}
	if out, err := exec.Command("mkfs.ext4", "-d", staging, "-F", img).CombinedOutput(); err != nil {
		return fmt.Errorf("mkfs.ext4: %s: %w", out, err)
	}
	return nil
}

// MetadataJSON helper for tests
func MetadataJSON(env map[string]string, mounts []map[string]string) string {
	b, _ := json.Marshal(map[string]interface{}{"env": env, "mounts": mounts})
	return string(b)
}

// jailerCgroupVersion picks cgroup v1 when legacy hierarchies are mounted, else v2.
func jailerCgroupVersion() string {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return "2"
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 1 && f[0] == "cgroup" {
			return "1"
		}
	}
	return "2"
}
