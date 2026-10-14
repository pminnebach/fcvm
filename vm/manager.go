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
	ops hostOps
}

// hostOps are the calls that need root, KVM or a real Firecracker. They are
// fields so lifecycle tests can drive the Manager against a temp state
// directory without touching the machine they run on.
type hostOps struct {
	euid            func() int
	launch          launchFunc
	patchRootfs     func(path string, opts rootfs.PatchOptions) error
	defaultIface    func() (string, error)
	enableIPForward func(stateDir string) error
	waitSSH         func(ctx context.Context, host, keyPath string, timeout time.Duration) error
	pushMounts      func(host, keyPath, table string) error
}

func realHostOps() hostOps {
	return hostOps{
		euid:            os.Geteuid,
		launch:          launchFirecracker,
		patchRootfs:     assets.PatchExt4,
		defaultIface:    network.DefaultIface,
		enableIPForward: network.EnableIPForward,
		waitSSH:         guest.WaitSSH,
		pushMounts: func(host, keyPath, table string) error {
			if err := guest.WriteFile(host, keyPath, "/etc/fcvm/mounts", table); err != nil {
				return fmt.Errorf("write guest mount table: %w", err)
			}
			if err := guest.Exec(host, keyPath, []string{"/usr/local/bin/fcvm-apply-mounts.sh"}); err != nil {
				return fmt.Errorf("apply guest mounts: %w", err)
			}
			return nil
		},
	}
}

func NewManager(cfg config.Config) *Manager {
	log := logrus.New()
	if cfg.Verbose {
		log.SetLevel(logrus.DebugLevel)
	}
	return &Manager{cfg: cfg, log: logrus.NewEntry(log), ops: realHostOps()}
}

// withConfig returns a Manager that behaves identically but with cfg, so a
// restart can run every existing step against the VM's saved shape.
func (m *Manager) withConfig(cfg config.Config) *Manager {
	cp := *m
	cp.cfg = cfg
	return &cp
}

func (m *Manager) requireRoot() error {
	if m.ops.euid() != 0 {
		return fmt.Errorf("fcvm must run as root (jailer, networking, and NFS require it)")
	}
	return nil
}

// launched is a booted Firecracker process, as far as the lifecycle cares.
type launched struct {
	pid         func() (int, error) // fallback when the jailer pid file is unreadable
	socketPath  string
	guestIP     string // CNI only: resolved after boot
	gateway     string
	mac         string
	setMetadata func(ctx context.Context, meta interface{}) error
	stop        func() error
	wait        func(ctx context.Context) error
}

type launchFunc func(ctx context.Context, cfg firecracker.Config, log *logrus.Entry, cni bool) (*launched, error)

func launchFirecracker(ctx context.Context, cfg firecracker.Config, log *logrus.Entry, cni bool) (*launched, error) {
	machine, err := firecracker.NewMachine(ctx, cfg, firecracker.WithLogger(log))
	if err != nil {
		return nil, fmt.Errorf("create machine: %w", err)
	}
	if err := machine.Start(ctx); err != nil {
		return nil, fmt.Errorf("start machine: %w", err)
	}
	l := &launched{
		pid:         machine.PID,
		socketPath:  machine.Cfg.SocketPath,
		setMetadata: machine.SetMetadata,
		stop:        machine.StopVMM,
		wait:        machine.Wait,
	}
	if cni {
		if l.guestIP, l.gateway, l.mac, err = resolveCNIAddrs(machine); err != nil {
			_ = machine.StopVMM()
			return nil, err
		}
	}
	return l, nil
}

// pendingMount is a mount whose guest-side record is only known once the VM
// has an address (NFS) or a drive letter (block).
type pendingMount struct {
	cfg       config.MountConfig
	guestPath string
	slot      int
	device    string // block only
}

// netSlot is the network identity a VM boots with.
type netSlot struct {
	index                  int
	useCNI                 bool
	tapDev, tapIP, guestIP string
	guestMAC, hostIface    string
	guestSubnet            string
	jailerUID, jailerGID   int
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

	if info, err := Inspect(m.cfg.StateDir, id); err == nil {
		return m.startExisting(ctx, info, unlock)
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
	slot := netSlot{index: index, useCNI: useCNI, jailerUID: uid, jailerGID: gid}

	if !useCNI {
		slot.hostIface, err = m.ops.defaultIface()
		if err != nil {
			return nil, err
		}
		var rebased bool
		slot.tapIP, slot.guestIP, rebased, err = network.ResolveTapAddrs(m.cfg.Network.TapIP, m.cfg.Network.GuestIP, index)
		if err != nil {
			return nil, err
		}
		if rebased {
			oldTap, oldGuest, _ := network.SubnetForIndex(m.cfg.Network.TapIP, m.cfg.Network.GuestIP, index)
			oldSubnet, _ := network.GuestSubnet(oldTap)
			newSubnet, _ := network.GuestSubnet(slot.tapIP)
			m.log.Warnf("guest subnet %s collides with the host; using %s (%s/%s) instead of %s/%s",
				oldSubnet, newSubnet, slot.tapIP, slot.guestIP, oldTap, oldGuest)
		}
		slot.tapDev = network.TapDevName(index)
		slot.guestMAC = network.GuestMAC(slot.guestIP)
		if slot.guestSubnet, err = network.GuestSubnet(slot.tapIP); err != nil {
			return nil, err
		}
	}

	// The configuration record claims the index on disk and is the shape every
	// later start of this VM reuses.
	if err := SaveVMConfig(m.cfg.StateDir, &VMConfig{
		ID:          id,
		Index:       index,
		Config:      m.cfg,
		TapDev:      slot.tapDev,
		TapIP:       slot.tapIP,
		GuestIP:     slot.guestIP,
		GuestMAC:    slot.guestMAC,
		GuestSubnet: slot.guestSubnet,
		JailerUID:   uid,
		JailerGID:   gid,
		CreatedAt:   time.Now(),
	}); err != nil {
		_ = RemoveState(m.cfg.StateDir, id)
		return nil, err
	}

	// A failed creation removes the whole VM; nothing of it was ever running.
	fail := func(err error) (*State, error) {
		m.rollback(id, slot, true)
		return nil, err
	}

	rootfsCopy := filepath.Join(vmDir, "rootfs.ext4")
	if err := copyFile(m.cfg.Rootfs, rootfsCopy); err != nil {
		return fail(fmt.Errorf("copy rootfs: %w", err))
	}
	patchOpts := rootfs.PatchOptions{
		SSHPubKey:     key.PublicKey,
		Env:           m.cfg.Env,
		Nameservers:   m.cfg.Network.Nameservers,
		StaticNetwork: !useCNI,
		GuestIP:       slot.guestIP,
		Gateway:       slot.tapIP,
	}
	if m.cfg.EnableVsock {
		agentPath, err := config.ResolveGuestAgent(m.cfg)
		if err != nil {
			return fail(err)
		}
		patchOpts.GuestAgentPath = agentPath
	}
	if err := m.ops.patchRootfs(rootfsCopy, patchOpts); err != nil {
		return fail(fmt.Errorf("patch rootfs: %w", err))
	}
	if err := m.chownForJailer(rootfsCopy, uid, gid); err != nil {
		return fail(err)
	}

	if err := m.setupHostNetwork(slot); err != nil {
		return fail(err)
	}

	// Block images must exist before the machine config is built; NFS exports
	// wait until the guest address is known so they can be scoped to it.
	blockDrives, pending, err := m.preparePending(vmDir, uid, gid, false)
	if err != nil {
		return fail(err)
	}

	return m.boot(ctx, bootPlan{
		id: id, slot: slot, keyPath: key.PrivateKeyPath, rootfsPath: rootfsCopy,
		blockDrives: blockDrives, pending: pending,
		rollback: func() { m.rollback(id, slot, true) },
	}, unlock)
}

// startExisting boots a stopped or crashed VM from its preserved disk using
// the configuration it was created with. Nothing about its shape may change,
// so the disk it was provisioned on stays valid.
func (m *Manager) startExisting(ctx context.Context, info *Info, unlock func()) (*State, error) {
	id := info.ID
	switch info.Status {
	case StatusRunning:
		return nil, fmt.Errorf("VM %q is already running", id)
	case StatusLegacy:
		return nil, errLegacy(id)
	}
	vc := info.Config
	em := m.withConfig(shapeFrom(m.cfg, vc.Config))
	if err := em.cfg.Validate(); err != nil {
		return nil, err
	}

	for _, bin := range []string{em.cfg.FirecrackerBin, em.cfg.JailerBin, em.cfg.Kernel} {
		if _, err := os.Stat(bin); err != nil {
			return nil, fmt.Errorf("missing %q: %w", bin, err)
		}
	}
	vmDir := em.cfg.VMStateDir(id)
	rootfsPath := filepath.Join(vmDir, "rootfs.ext4")
	if _, err := os.Stat(rootfsPath); err != nil {
		return nil, fmt.Errorf("VM %q has no disk to start from: %w", id, err)
	}

	// A crash leaves its TAP device, rules, exports and jail behind. Take them
	// down before the same slot is set up again.
	if info.State != nil {
		em.teardownState(info.State)
		if err := removeRuntimeState(em.cfg.StateDir, id); err != nil {
			return nil, err
		}
	}
	em.removeJailerTree(id)
	if err := os.MkdirAll(em.cfg.Jailer.ChrootBaseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create jailer chroot base %q: %w", em.cfg.Jailer.ChrootBaseDir, err)
	}

	key, err := guest.LoadOrCreateKey(em.cfg.SSHKey)
	if err != nil {
		return nil, err
	}

	slot := netSlot{
		index:       vc.Index,
		useCNI:      vc.IsCNI(),
		tapDev:      vc.TapDev,
		tapIP:       vc.TapIP,
		guestIP:     vc.GuestIP,
		guestMAC:    vc.GuestMAC,
		guestSubnet: vc.GuestSubnet,
		jailerUID:   vc.JailerUID,
		jailerGID:   vc.JailerGID,
	}
	if !slot.useCNI {
		if slot.hostIface, err = em.ops.defaultIface(); err != nil {
			return nil, err
		}
	}

	// A failed restart gives back what this attempt took and leaves the VM
	// stopped; its disk is what the user came back for.
	fail := func(err error) (*State, error) {
		em.rollback(id, slot, false)
		return nil, err
	}

	if err := em.chownForJailer(rootfsPath, slot.jailerUID, slot.jailerGID); err != nil {
		return fail(err)
	}
	if err := em.setupHostNetwork(slot); err != nil {
		return fail(err)
	}
	blockDrives, pending, err := em.preparePending(vmDir, slot.jailerUID, slot.jailerGID, true)
	if err != nil {
		return fail(err)
	}

	return em.boot(ctx, bootPlan{
		id: id, slot: slot, keyPath: key.PrivateKeyPath, rootfsPath: rootfsPath,
		blockDrives: blockDrives, pending: pending,
		rollback: func() { em.rollback(id, slot, false) },
		started:  func() error { return removeStopRecord(em.cfg.StateDir, id) },
	}, unlock)
}

// shapeFrom overlays the parts of a VM's saved configuration that define what
// it is onto the current configuration, which still supplies where things live
// on this host (binaries, kernel, state directory, timeouts).
func shapeFrom(cur, saved config.Config) config.Config {
	cur.KernelArgs = saved.KernelArgs
	cur.LogLevel = saved.LogLevel
	cur.CPUTemplate = saved.CPUTemplate
	cur.DisableSMT = saved.DisableSMT
	cur.VCPUCount = saved.VCPUCount
	cur.MemSizeMib = saved.MemSizeMib
	cur.Network = saved.Network
	cur.Env = saved.Env
	cur.Mounts = saved.Mounts
	cur.SSHKey = saved.SSHKey
	cur.EnableVsock = saved.EnableVsock
	base := cur.Jailer.ChrootBaseDir
	cur.Jailer = saved.Jailer
	cur.Jailer.ChrootBaseDir = base
	return cur
}

// setupHostNetwork brings up the TAP device and firewall rules for a slot.
// CNI VMs get their network from the SDK at launch instead.
func (m *Manager) setupHostNetwork(s netSlot) error {
	if s.useCNI {
		return nil
	}
	if err := m.ops.enableIPForward(m.cfg.StateDir); err != nil {
		return fmt.Errorf("enable ip forwarding: %w", err)
	}
	if err := network.SetupTap(s.tapDev, s.tapIP, s.guestIP, s.hostIface); err != nil {
		m.releaseHostNetwork()
		return err
	}
	return nil
}

// rollback undoes a failed start. removeVM distinguishes a VM that never
// existed (remove everything) from one that is being restarted (keep it).
func (m *Manager) rollback(id string, s netSlot, removeVM bool) {
	rt := &State{ID: id, TapDev: s.tapDev, GuestSubnet: s.guestSubnet, HostIface: s.hostIface}
	if s.useCNI {
		rt.NetworkMode = NetworkModeCNI
		rt.CNINetwork = m.cfg.Network.CNINetwork
	}
	m.releaseRuntime(rt)
	if removeVM {
		_ = RemoveState(m.cfg.StateDir, id)
	}
	// A failed start must not leave the host chain and ip_forward behind.
	m.releaseHostNetwork()
}

// preparePending resolves the configured mounts. A new VM builds its block
// images from the host directories; an existing VM reuses the images it has,
// which hold the guest's changes.
func (m *Manager) preparePending(vmDir string, uid, gid int, reuseBlock bool) ([]blockDrive, []pendingMount, error) {
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
				return nil, nil, fmt.Errorf("at most %d block mounts are supported per VM", maxBlockDrives)
			}
			img := filepath.Join(vmDir, fmt.Sprintf("mount-%d.ext4", i))
			if reuseBlock {
				if _, err := os.Stat(img); err != nil {
					return nil, nil, fmt.Errorf("block mount image: %w", err)
				}
			} else if err := syncDirToExt4(mount.Host, img, mount.Size); err != nil {
				return nil, nil, err
			}
			if err := m.chownForJailer(img, uid, gid); err != nil {
				return nil, nil, err
			}
			p.device = guestBlockDevice(len(blockDrives))
			blockDrives = append(blockDrives, blockDrive{Path: img, ReadOnly: mount.ReadOnly()})
		}
		pending = append(pending, p)
	}
	return blockDrives, pending, nil
}

// bootPlan is everything boot needs once the disk and host network are ready.
type bootPlan struct {
	id          string
	slot        netSlot
	keyPath     string
	rootfsPath  string
	blockDrives []blockDrive
	pending     []pendingMount
	rollback    func()
	// started runs once the runtime state is saved (a restart clears the stop
	// record here, so a failure earlier leaves the VM stopped).
	started func() error
}

// boot launches Firecracker, wires up mounts, records the runtime state and
// waits for the guest. It is the shared tail of creating and restarting a VM.
func (m *Manager) boot(ctx context.Context, p bootPlan, unlock func()) (*State, error) {
	id, slot := p.id, p.slot
	fcCfg := buildFirecrackerConfig(m.cfg, machineBuildInput{
		ID:          id,
		RootfsPath:  p.rootfsPath,
		BlockDrives: p.blockDrives,
		TapDev:      slot.tapDev,
		TapIP:       slot.tapIP,
		GuestIP:     slot.guestIP,
		GuestMAC:    slot.guestMAC,
		JailerUID:   slot.jailerUID,
		JailerGID:   slot.jailerGID,
	})

	l, err := m.ops.launch(ctx, fcCfg, m.log, slot.useCNI)
	if err != nil {
		p.rollback()
		return nil, err
	}
	stopAndFail := func(err error) (*State, error) {
		_ = l.stop()
		p.rollback()
		return nil, err
	}

	guestIP, gateway, guestMAC := slot.guestIP, slot.tapIP, slot.guestMAC
	if slot.useCNI {
		guestIP, gateway, guestMAC = l.guestIP, l.gateway, l.mac
	}

	mountStates, mountRecords, err := m.setupMounts(id, guestIP, gateway, p.pending)
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
	if metaErr := l.setMetadata(ctx, meta); metaErr != nil {
		m.log.Warnf("set MMDS metadata: %v", metaErr)
	}

	chrootDir := filepath.Join(m.cfg.Jailer.ChrootBaseDir, "firecracker", id, "root")
	// Prefer the jailer's firecracker.pid: with --daemonize the process
	// Machine.PID() returns is the exiting parent, not Firecracker.
	pid, err := readJailerPIDFile(chrootDir, m.cfg.FirecrackerBin)
	if err != nil {
		fallback, pidErr := l.pid()
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
	if slot.useCNI {
		netMode = NetworkModeCNI
		cniName = m.cfg.Network.CNINetwork
	}

	state := &State{
		ID:          id,
		Index:       slot.index,
		PID:         pid,
		PIDStart:    pidStart,
		SocketPath:  l.socketPath,
		NetworkMode: netMode,
		CNINetwork:  cniName,
		TapDev:      slot.tapDev,
		HostIface:   slot.hostIface,
		GuestSubnet: slot.guestSubnet,
		GuestIP:     guestIP,
		GuestMAC:    guestMAC,
		SSHKey:      p.keyPath,
		ChrootDir:   chrootDir,
		LogPath:     filepath.Join(chrootDir, fcCfg.LogPath),
		JailerUID:   slot.jailerUID,
		JailerGID:   slot.jailerGID,
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
	if p.started != nil {
		if err := p.started(); err != nil {
			_ = removeRuntimeState(m.cfg.StateDir, id)
			return stopAndFail(err)
		}
	}
	// The index is claimed on disk now, so other starts can proceed while this
	// one waits for the guest to come up.
	unlock()

	timeout := time.Duration(m.cfg.WaitTimeoutSec) * time.Second
	if err := m.ops.waitSSH(ctx, guestIP, p.keyPath, timeout); err != nil {
		m.log.Warnf("guest ssh not ready: %v", err)
	} else if len(mountRecords) > 0 {
		// The host knows the whole mount table; push it and let the guest
		// script consume it, rather than parsing JSON in shell.
		if err := m.ops.pushMounts(guestIP, p.keyPath, rootfs.RenderMounts(mountRecords)); err != nil {
			m.log.Warnf("%v", err)
		}
	}

	go m.waitVM(context.WithoutCancel(ctx), l, id)

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

func (m *Manager) waitVM(ctx context.Context, l *launched, id string) {
	_ = l.wait(ctx)
	m.log.Infof("VM %q exited", id)
}

// Stop halts a VM and releases what it holds on the host, but keeps its disk,
// configuration and network slot so Start can bring it back. A VM that is
// already stopped is left alone.
func (m *Manager) Stop(id string) error {
	if err := m.requireRoot(); err != nil {
		return err
	}
	info, err := Inspect(m.cfg.StateDir, id)
	if err != nil {
		return err
	}
	switch info.Status {
	case StatusStopped:
		return nil
	case StatusLegacy:
		// Nothing to come back to: it has no configuration record.
		m.cleanupVM(id, info.State)
		m.releaseHostNetwork()
		return nil
	}
	return m.stopVM(info)
}

// stopVM stops a running or crashed VM. The stop record is written before the
// runtime state goes, so a stop that is interrupted partway still reads as a
// crash rather than as a stop that did not finish.
func (m *Manager) stopVM(info *Info) error {
	state := info.State
	m.stopVMProcess(state)
	m.syncBlockMounts(state)
	m.releaseRuntime(state)
	if err := SaveStopRecord(m.cfg.StateDir, info.ID, time.Now()); err != nil {
		return err
	}
	if err := removeRuntimeState(m.cfg.StateDir, info.ID); err != nil {
		return err
	}
	m.releaseHostNetwork()
	return nil
}

// Restart stops the VM if it is running and starts it again. It never creates
// a VM: an unknown id is an error.
func (m *Manager) Restart(ctx context.Context, id string) (*State, error) {
	if err := m.requireRoot(); err != nil {
		return nil, err
	}
	info, err := Inspect(m.cfg.StateDir, id)
	if err != nil {
		return nil, err
	}
	if info.Status == StatusLegacy {
		return nil, errLegacy(id)
	}
	if info.Status == StatusRunning {
		if err := m.stopVM(info); err != nil {
			return nil, err
		}
	}
	return m.Start(ctx, id)
}

func errLegacy(id string) error {
	return fmt.Errorf("VM %q was created before VMs could be restarted; run `fcvm cleanup %s` and create it again", id, id)
}

// Delete removes a VM for good: its disk, configuration and network slot. A
// running VM is refused unless force is set, in which case it is stopped first.
func (m *Manager) Delete(id string, force bool) error {
	if err := m.requireRoot(); err != nil {
		return err
	}
	info, err := Inspect(m.cfg.StateDir, id)
	if err != nil {
		return err
	}
	if info.Status == StatusRunning && !force {
		return fmt.Errorf("VM %q is running; stop it first or use --force", id)
	}
	m.cleanupVM(id, info.State)
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

// cleanupVM removes a VM completely, with or without a state record to guide
// it. Without one the TAP name is unknown; exports and the jailer tree are
// still derivable from the id.
func (m *Manager) cleanupVM(id string, state *State) {
	if state != nil {
		m.stopVMProcess(state)
		m.syncBlockMounts(state)
	} else {
		state = &State{ID: id}
	}
	m.releaseRuntime(state)
	_ = RemoveState(m.cfg.StateDir, id)
}

// releaseHostNetwork undoes host-wide changes once no VM is running. Stopped
// VMs hold no TAP device or rules, so they do not keep the chain open.
func (m *Manager) releaseHostNetwork() {
	infos, err := ListInfos(m.cfg.StateDir)
	if err != nil {
		return
	}
	for _, info := range infos {
		if info.Status == StatusRunning {
			return
		}
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

// List returns every VM with its status.
func (m *Manager) List() ([]Info, error) {
	return ListInfos(m.cfg.StateDir)
}

// nextVMIndex returns the lowest index no VM has claimed. A stopped VM keeps
// its index (and with it its TAP subnet, guest IP and jailer uid) so that it
// can come back on the same address; counting VMs, or only the running ones,
// would hand that slot to a new VM.
func (m *Manager) nextVMIndex() (int, error) {
	states, err := ListStates(m.cfg.StateDir)
	if err != nil {
		return 0, err
	}
	claimed := ClaimedIndices(states)
	infos, err := ListInfos(m.cfg.StateDir)
	if err != nil {
		return 0, err
	}
	for _, info := range infos {
		if info.Config != nil {
			claimed[info.Config.Index] = true
		}
	}
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

// releaseRuntime gives back what a running (or half-started) VM holds on the
// host: network, NFS exports, jail and socket. It never touches the VM's disk
// or its records. Stop, delete, cleanup, crash recovery and failed-start
// rollback all go through here so a new host resource is released in one place.
func (m *Manager) releaseRuntime(rt *State) {
	m.teardownState(rt)
	m.removeJailerTree(rt.ID)
}
