package vm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/sirupsen/logrus"

	"github.com/pminnebach/fcvm/assets"
	"github.com/pminnebach/fcvm/config"
	"github.com/pminnebach/fcvm/guest"
	"github.com/pminnebach/fcvm/network"
	"github.com/pminnebach/fcvm/rootfs"
	"github.com/pminnebach/fcvm/vsock"
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

// pendingMount is a mount whose guest-side record is only known once the VM
// has an address (NFS) or a drive letter (block).
type pendingMount struct {
	cfg       config.MountConfig
	guestPath string
	slot      int
	device    string // block only
}

func (m *Manager) Start(ctx context.Context, id string) (*State, error) {
	if err := m.requireRoot(); err != nil {
		return nil, err
	}
	if id == "" {
		id = "vm-" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	if err := m.cfg.Validate(); err != nil {
		return nil, err
	}

	// Index allocation and the state write that claims it must not interleave
	// with another start, or both VMs get the same TAP name and address.
	unlock, err := lockState(m.cfg.StateDir)
	if err != nil {
		return nil, err
	}
	defer unlock()

	if _, err := LoadState(m.cfg.StateDir, id); err == nil {
		return nil, fmt.Errorf("VM %q already exists; stop it first", id)
	}
	m.removeJailerTree(id)

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

	useCNI := m.cfg.Network.CNINetwork != ""
	index, err := m.nextVMIndex()
	if err != nil {
		return nil, err
	}
	uid, gid := jailerCreds(m.cfg, index)

	var tapIP, guestIP, tapDev, guestMAC, hostIface string
	if !useCNI {
		hostIface, err = network.DefaultIface()
		if err != nil {
			return nil, err
		}
		var rebased bool
		tapIP, guestIP, rebased, err = network.ResolveTapAddrs(m.cfg.Network.TapIP, m.cfg.Network.GuestIP, index)
		if err != nil {
			return nil, err
		}
		if rebased {
			oldTap, oldGuest, _ := network.SubnetForIndex(m.cfg.Network.TapIP, m.cfg.Network.GuestIP, index)
			oldSubnet, _ := network.GuestSubnet(oldTap)
			newSubnet, _ := network.GuestSubnet(tapIP)
			m.log.Warnf("guest subnet %s collides with the host; using %s (%s/%s) instead of %s/%s",
				oldSubnet, newSubnet, tapIP, guestIP, oldTap, oldGuest)
		}
		tapDev = network.TapDevName(index)
		guestMAC = network.GuestMAC(guestIP)
	}

	rootfsCopy := filepath.Join(vmDir, "rootfs.ext4")
	if err := copyFile(m.cfg.Rootfs, rootfsCopy); err != nil {
		return nil, fmt.Errorf("copy rootfs: %w", err)
	}
	patchOpts := rootfs.PatchOptions{
		SSHPubKey:     key.PublicKey,
		Env:           m.cfg.Env,
		Nameservers:   m.cfg.Network.Nameservers,
		StaticNetwork: !useCNI,
		GuestIP:       guestIP,
		Gateway:       tapIP,
	}
	if m.cfg.EnableVsock {
		agentPath, err := config.ResolveGuestAgent(m.cfg)
		if err != nil {
			_ = RemoveState(m.cfg.StateDir, id)
			return nil, err
		}
		patchOpts.GuestAgentPath = agentPath
	}
	if err := assets.PatchExt4(rootfsCopy, patchOpts); err != nil {
		_ = RemoveState(m.cfg.StateDir, id)
		return nil, fmt.Errorf("patch rootfs: %w", err)
	}
	if err := m.chownForJailer(rootfsCopy, uid, gid); err != nil {
		_ = RemoveState(m.cfg.StateDir, id)
		return nil, err
	}

	if !useCNI {
		if err := network.EnableIPForward(m.cfg.StateDir); err != nil {
			_ = RemoveState(m.cfg.StateDir, id)
			return nil, fmt.Errorf("enable ip forwarding: %w", err)
		}
		if err := network.SetupTap(tapDev, tapIP, guestIP, hostIface); err != nil {
			_ = RemoveState(m.cfg.StateDir, id)
			m.releaseHostNetwork()
			return nil, err
		}
	}

	guestSubnet := ""
	if !useCNI {
		if guestSubnet, err = network.GuestSubnet(tapIP); err != nil {
			_ = RemoveState(m.cfg.StateDir, id)
			return nil, err
		}
	}

	failCleanup := func() {
		if useCNI {
			_ = network.TeardownCNI(context.Background(), id, m.cfg.Network.CNINetwork)
		} else {
			network.TeardownTap(tapDev, guestSubnet, hostIface)
		}
		network.TeardownNFSExportsForVM(m.cfg.ExportRoot(), id)
		m.removeJailerTree(id)
		_ = RemoveState(m.cfg.StateDir, id)
		// A failed start must not leave the host chain and ip_forward behind.
		m.releaseHostNetwork()
	}

	// Block images must exist before the machine config is built; NFS exports
	// wait until the guest address is known so they can be scoped to it.
	var blockDrives []blockDrive
	var pending []pendingMount
	for i, mount := range m.cfg.Mounts {
		guestPath := mount.Guest
		if guestPath == "" {
			guestPath = "/mnt/" + filepath.Base(mount.Host)
		}
		p := pendingMount{cfg: mount, guestPath: guestPath, slot: i}
		if mount.ResolvedMethod() == config.MountBlock {
			if len(blockDrives) >= maxBlockDrives {
				failCleanup()
				return nil, fmt.Errorf("at most %d block mounts are supported per VM", maxBlockDrives)
			}
			img := filepath.Join(vmDir, fmt.Sprintf("mount-%d.ext4", i))
			if err := syncDirToExt4(mount.Host, img, mount.Size); err != nil {
				failCleanup()
				return nil, err
			}
			if err := m.chownForJailer(img, uid, gid); err != nil {
				failCleanup()
				return nil, err
			}
			p.device = guestBlockDevice(len(blockDrives))
			blockDrives = append(blockDrives, blockDrive{Path: img, ReadOnly: mount.ReadOnly()})
		}
		pending = append(pending, p)
	}

	fcCfg := buildFirecrackerConfig(m.cfg, machineBuildInput{
		ID:          id,
		RootfsPath:  rootfsCopy,
		BlockDrives: blockDrives,
		TapDev:      tapDev,
		TapIP:       tapIP,
		GuestIP:     guestIP,
		GuestMAC:    guestMAC,
		JailerUID:   uid,
		JailerGID:   gid,
	})

	machine, err := firecracker.NewMachine(ctx, fcCfg, firecracker.WithLogger(m.log))
	if err != nil {
		failCleanup()
		return nil, fmt.Errorf("create machine: %w", err)
	}

	if err := machine.Start(ctx); err != nil {
		failCleanup()
		return nil, fmt.Errorf("start machine: %w", err)
	}

	stopAndFail := func(err error) (*State, error) {
		_ = machine.StopVMM()
		failCleanup()
		return nil, err
	}

	gateway := tapIP
	if useCNI {
		resolvedIP, resolvedGW, resolvedMAC, err := resolveCNIAddrs(machine)
		if err != nil {
			return stopAndFail(err)
		}
		guestIP, gateway, guestMAC = resolvedIP, resolvedGW, resolvedMAC
	}

	mountStates, mountRecords, err := m.setupMounts(id, guestIP, gateway, pending)
	if err != nil {
		return stopAndFail(err)
	}

	meta := map[string]interface{}{
		"latest": map[string]interface{}{
			"meta-data": map[string]interface{}{
				"env": m.cfg.Env,
			},
		},
	}
	if metaErr := machine.SetMetadata(ctx, meta); metaErr != nil {
		m.log.Warnf("set MMDS metadata: %v", metaErr)
	}

	chrootDir := filepath.Join(m.cfg.Jailer.ChrootBaseDir, "firecracker", id, "root")
	// Prefer the jailer's firecracker.pid: with --daemonize the process
	// Machine.PID() returns is the exiting parent, not Firecracker.
	pid, err := readJailerPIDFile(chrootDir, m.cfg.FirecrackerBin)
	if err != nil {
		fallback, pidErr := machine.PID()
		if pidErr != nil {
			m.log.Warnf("firecracker pid file: %v; machine.PID: %v", err, pidErr)
		} else {
			m.log.Warnf("read firecracker.pid: %v; falling back to machine.PID %d", err, fallback)
			pid = fallback
		}
	}
	pidStart, err := procStartTime(pid)
	if err != nil {
		// Without it, IsRunning falls back to PID-only liveness, which cannot
		// tell this VM from a process that later reuses its PID.
		m.log.Warnf("record start time for pid %d: %v; stop will rely on the PID alone", pid, err)
	}

	netMode := NetworkModeTAP
	cniName := ""
	if useCNI {
		netMode = NetworkModeCNI
		cniName = m.cfg.Network.CNINetwork
	}

	state := &State{
		ID:          id,
		Index:       index,
		PID:         pid,
		PIDStart:    pidStart,
		SocketPath:  machine.Cfg.SocketPath,
		NetworkMode: netMode,
		CNINetwork:  cniName,
		TapDev:      tapDev,
		HostIface:   hostIface,
		GuestSubnet: guestSubnet,
		GuestIP:     guestIP,
		GuestMAC:    guestMAC,
		SSHKey:      key.PrivateKeyPath,
		ChrootDir:   chrootDir,
		LogPath:     filepath.Join(chrootDir, fcCfg.LogPath),
		JailerUID:   uid,
		JailerGID:   gid,
		StartedAt:   time.Now(),
		Mounts:      mountStates,
		Env:         m.cfg.Env,
	}
	if m.cfg.EnableVsock {
		state.VsockUDS = filepath.Join(chrootDir, vsock.UDSName)
		state.VsockCID = vsock.GuestCID
	}
	if err := SaveState(m.cfg.StateDir, state); err != nil {
		return stopAndFail(err)
	}
	// The index is claimed on disk now, so other starts can proceed while this
	// one waits for the guest to come up.
	unlock()

	timeout := time.Duration(m.cfg.WaitTimeoutSec) * time.Second
	if err := guest.WaitSSH(ctx, guestIP, key.PrivateKeyPath, timeout); err != nil {
		m.log.Warnf("guest ssh not ready: %v", err)
	} else if len(mountRecords) > 0 {
		// The host knows the whole mount table; push it and let the guest
		// script consume it, rather than parsing JSON in shell.
		table := rootfs.RenderMounts(mountRecords)
		if err := guest.WriteFile(guestIP, key.PrivateKeyPath, "/etc/fcvm/mounts", table); err != nil {
			m.log.Warnf("write guest mount table: %v", err)
		} else if err := guest.Exec(guestIP, key.PrivateKeyPath, []string{"/usr/local/bin/fcvm-apply-mounts.sh"}); err != nil {
			m.log.Warnf("apply guest mounts: %v", err)
		}
	}

	go m.waitVM(context.WithoutCancel(ctx), machine, id)

	return state, nil
}

// setupMounts creates NFS exports now that the guest address is known, and
// returns the persisted mount state plus the table pushed into the guest.
func (m *Manager) setupMounts(id, guestIP, gateway string, pending []pendingMount) ([]MountState, []rootfs.MountRecord, error) {
	var states []MountState
	var records []rootfs.MountRecord
	for _, p := range pending {
		switch p.cfg.ResolvedMethod() {
		case config.MountBlock:
			states = append(states, MountState{
				Host:     p.cfg.Host,
				Guest:    p.guestPath,
				Method:   config.MountBlock,
				Device:   filepath.Join(m.cfg.VMStateDir(id), fmt.Sprintf("mount-%d.ext4", p.slot)),
				ReadOnly: p.cfg.ReadOnly(),
			})
			records = append(records, rootfs.MountRecord{
				Method: config.MountBlock, Source: p.device, Guest: p.guestPath,
			})
		default:
			if guestIP == "" {
				return nil, nil, fmt.Errorf("mount %q: guest address unknown, cannot scope NFS export", p.cfg.Host)
			}
			exportID := id + "-" + strconv.Itoa(p.slot)
			exp, err := network.SetupNFSExport(m.cfg.ExportRoot(), p.cfg.Host, exportID, guestIP, p.cfg.ReadOnly())
			if err != nil {
				return nil, nil, fmt.Errorf("mount %q over NFS: %w (use method=block to copy the directory into the VM instead)", p.cfg.Host, err)
			}
			server := gateway
			if server == "" {
				server = guestIP
			}
			states = append(states, MountState{
				Host:     exp.ExportPath,
				Guest:    p.guestPath,
				Method:   config.MountNFS,
				ReadOnly: p.cfg.ReadOnly(),
			})
			records = append(records, rootfs.MountRecord{
				Method: config.MountNFS, Source: server + ":" + exp.ExportPath, Guest: p.guestPath,
			})
		}
	}
	return states, records, nil
}

// maxBlockDrives keeps guest device names within /dev/vdb../dev/vdz.
const maxBlockDrives = 25

// guestBlockDevice maps a data drive slot to its guest device node. The root
// filesystem takes /dev/vda, so data drives start at /dev/vdb.
func guestBlockDevice(slot int) string {
	return "/dev/vd" + string(rune('b'+slot))
}

func resolveCNIAddrs(machine *firecracker.Machine) (guestIP, gateway, mac string, err error) {
	if len(machine.Cfg.NetworkInterfaces) == 0 {
		return "", "", "", fmt.Errorf("CNI: no network interfaces after start")
	}
	iface := machine.Cfg.NetworkInterfaces[0]
	if iface.StaticConfiguration == nil {
		return "", "", "", fmt.Errorf("CNI: StaticConfiguration not filled after start (need tc-redirect-tap)")
	}
	mac = iface.StaticConfiguration.MacAddress
	ipCfg := iface.StaticConfiguration.IPConfiguration
	if ipCfg == nil || ipCfg.IPAddr.IP == nil {
		return "", "", "", fmt.Errorf("CNI: no guest IP in CNI result")
	}
	guestIP = ipCfg.IPAddr.IP.String()
	if ipCfg.Gateway != nil {
		gateway = ipCfg.Gateway.String()
	}
	return guestIP, gateway, mac, nil
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
	m.stopVMProcess(state)
	m.syncBlockMounts(state)
	m.teardownState(state)
	if err := RemoveState(m.cfg.StateDir, id); err != nil {
		return err
	}
	m.releaseHostNetwork()
	return nil
}

// stopVMProcess signals the VM and waits for it to go away. It never signals a
// PID it cannot still identify as this VM: PIDs are recycled, and fcvm runs as
// root.
func (m *Manager) stopVMProcess(state *State) {
	if !state.IsRunning() {
		return
	}
	if err := syscall.Kill(state.PID, syscall.SIGTERM); err != nil {
		return
	}
	timeout := time.Duration(m.cfg.StopTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !state.IsRunning() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	if state.IsRunning() {
		m.log.Warnf("VM %q did not exit within %s; sending SIGKILL", state.ID, timeout)
		_ = syscall.Kill(state.PID, syscall.SIGKILL)
	}
}

func (m *Manager) Cleanup(all bool, id string) error {
	if err := m.requireRoot(); err != nil {
		return err
	}
	if all {
		ids, err := ListVMDirIDs(m.cfg.StateDir)
		if err != nil {
			return err
		}
		for _, vmID := range ids {
			state, err := LoadState(m.cfg.StateDir, vmID)
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					m.log.Warnf("skip %q: %v", vmID, err)
					continue
				}
				m.cleanupVM(vmID, nil)
				continue
			}
			m.cleanupVM(vmID, state)
		}
		m.releaseHostNetwork()
		return nil
	}
	if id == "" {
		return fmt.Errorf("specify VM id or --all")
	}
	if err := ValidateID(id); err != nil {
		return err
	}
	state, err := LoadState(m.cfg.StateDir, id)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		m.cleanupVM(id, nil)
		m.releaseHostNetwork()
		return nil
	}
	m.cleanupVM(id, state)
	m.releaseHostNetwork()
	return nil
}

func (m *Manager) cleanupVM(id string, state *State) {
	if state != nil {
		m.stopVMProcess(state)
		m.syncBlockMounts(state)
		m.teardownState(state)
	} else {
		// Without state the tap name is unknown; exports and the jailer tree
		// are still derivable from the id.
		network.TeardownNFSExportsForVM(m.cfg.ExportRoot(), id)
		m.removeJailerTree(id)
	}
	_ = RemoveState(m.cfg.StateDir, id)
}

// releaseHostNetwork undoes host-wide changes once the last VM is gone.
func (m *Manager) releaseHostNetwork() {
	states, err := ListStates(m.cfg.StateDir)
	if err != nil || len(states) > 0 {
		return
	}
	network.RemoveFCVMChain()
	network.RestoreIPForward(m.cfg.StateDir)
}

// syncBlockMounts copies writable block-device mounts back to their host
// directories. Without this the guest's writes are discarded when the VM's
// state directory is removed.
func (m *Manager) syncBlockMounts(state *State) {
	for _, mnt := range state.Mounts {
		if mnt.Method != config.MountBlock || mnt.ReadOnly || mnt.Device == "" || mnt.Host == "" {
			continue
		}
		if err := syncExt4ToDir(mnt.Device, mnt.Host); err != nil {
			m.log.Errorf("sync %s back to %s: %v", mnt.Device, mnt.Host, err)
		}
	}
}

func (m *Manager) chownForJailer(path string, uid, gid int) error {
	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("chown %q for jailer uid %d: %w", path, uid, err)
	}
	return nil
}

func (m *Manager) jailerTreeDir(id string) string {
	return filepath.Join(m.cfg.Jailer.ChrootBaseDir, "firecracker", id)
}

func (m *Manager) removeJailerTree(id string) {
	if err := ValidateID(id); err != nil {
		return
	}
	_ = os.RemoveAll(m.jailerTreeDir(id))
}

func (m *Manager) teardownState(state *State) {
	if state.IsCNI() {
		_ = network.TeardownCNI(context.Background(), state.ID, state.CNINetwork)
	} else if state.TapDev != "" {
		subnet, iface := state.GuestSubnet, state.HostIface
		// State written before these fields existed still has rules to remove;
		// recover them rather than leaving the rules behind.
		if subnet == "" {
			subnet, _ = network.GuestSubnet(state.GuestIP)
		}
		if iface == "" {
			iface, _ = network.DefaultIface()
		}
		network.TeardownTap(state.TapDev, subnet, iface)
	}
	network.TeardownNFSExportsForVM(m.cfg.ExportRoot(), state.ID)
	if state.ChrootDir != "" {
		_ = os.RemoveAll(filepath.Dir(state.ChrootDir))
	}
	_ = os.Remove(state.SocketPath)
}

func (m *Manager) List() ([]State, error) {
	return ListStates(m.cfg.StateDir)
}

// nextVMIndex returns the lowest index no live VM has claimed. Using the count
// of VMs would reuse the index of any stopped VM and collide with the ones
// still running.
func (m *Manager) nextVMIndex() (int, error) {
	states, err := ListStates(m.cfg.StateDir)
	if err != nil {
		return 0, err
	}
	claimed := ClaimedIndices(states)
	for i := 0; ; i++ {
		if !claimed[i] {
			return i, nil
		}
	}
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
	if _, err := out.ReadFrom(in); err != nil {
		return err
	}
	return out.Sync()
}

// syncDirToExt4 builds a block image seeded with the contents of hostDir.
func syncDirToExt4(hostDir, img, size string) error {
	if _, err := os.Stat(hostDir); err != nil {
		return fmt.Errorf("mount source %q: %w", hostDir, err)
	}
	dir, err := os.MkdirTemp("", "fcvm-sync-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	staging := filepath.Join(dir, "staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	if out, err := exec.Command("cp", "-a", hostDir+"/.", staging+"/").CombinedOutput(); err != nil {
		return fmt.Errorf("copy %s: %s: %w", hostDir, strings.TrimSpace(string(out)), err)
	}
	if size == "" {
		size, err = rootfs.SizeForDir(staging)
		if err != nil {
			return err
		}
	}
	return rootfs.MakeExt4(staging, img, size)
}

// syncExt4ToDir mirrors a block image back onto the host directory it came
// from. The image is unmounted before the temp dir is removed, and the removal
// is skipped if that unmount failed, because RemoveAll over a live mount would
// delete the user's data.
func syncExt4ToDir(img, hostDir string) (err error) {
	if _, statErr := os.Stat(img); statErr != nil {
		return nil // nothing to sync back
	}
	dir, mkErr := os.MkdirTemp("", "fcvm-writeback-*")
	if mkErr != nil {
		return mkErr
	}
	mountPoint := filepath.Join(dir, "mnt")
	defer func() {
		if network.IsMountPoint(mountPoint) {
			err = errors.Join(err, fmt.Errorf("%s still mounted; left %s in place", mountPoint, dir))
			return
		}
		_ = os.RemoveAll(dir)
	}()
	if err := os.Mkdir(mountPoint, 0o755); err != nil {
		return err
	}
	if out, mErr := exec.Command("mount", "-o", "loop", img, mountPoint).CombinedOutput(); mErr != nil {
		return fmt.Errorf("mount %s: %s: %w", img, strings.TrimSpace(string(out)), mErr)
	}
	defer func() {
		if out, uErr := exec.Command("umount", mountPoint).CombinedOutput(); uErr != nil {
			err = errors.Join(err, fmt.Errorf("umount %s: %s: %w", mountPoint, strings.TrimSpace(string(out)), uErr))
		}
	}()
	return mirrorDir(mountPoint, hostDir)
}

// mirrorDir makes dst match src: entries removed in src are deleted from dst,
// then everything is copied across preserving ownership, modes and times.
// Done with cp -a plus a prune rather than rsync, which is not installed
// everywhere and would make write-back fail exactly when data is at stake.
func mirrorDir(src, dst string) error {
	stale, err := staleEntries(src, dst)
	if err != nil {
		return err
	}
	for _, p := range stale {
		if err := os.RemoveAll(p); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == "lost+found" {
			continue // ext4 bookkeeping, not the user's data
		}
		if out, err := exec.Command("cp", "-a", filepath.Join(src, e.Name()), dst+"/").CombinedOutput(); err != nil {
			return fmt.Errorf("copy %s: %s: %w", e.Name(), strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}

// staleEntries lists paths under dst with no counterpart in src.
func staleEntries(src, dst string) ([]string, error) {
	var stale []string
	err := filepath.WalkDir(dst, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dst, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if _, err := os.Lstat(filepath.Join(src, rel)); errors.Is(err, os.ErrNotExist) {
			stale = append(stale, p)
			if d.IsDir() {
				return filepath.SkipDir
			}
		}
		return nil
	})
	return stale, err
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
