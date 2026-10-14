package vm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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

// --- waiting for the process (finding: stop returned right after SIGKILL) ---

func TestStopEscalatesAndWaitsForTheProcessToBeGone(t *testing.T) {
	m, h := newTestManager(t)
	m.cfg.StopTimeoutSec = 1
	h.ignoreTerm = true
	st := mustStart(t, m, "web")

	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	// Syncing or deleting the disk while the process still has it open is the
	// failure this guards against.
	if (&State{PID: st.PID, PIDStart: st.PIDStart}).IsRunning() {
		t.Fatal("stop returned while the VM process was still alive")
	}
}

func TestStopFailsAndChangesNothingWhenTheProcessWillNotDie(t *testing.T) {
	m, h := newTestManager(t)
	m.cfg.StopTimeoutSec = 1
	mustStart(t, m, "web")
	disk := filepath.Join(m.cfg.StateDir, "vms", "web", "rootfs.ext4")
	if err := os.WriteFile(disk, []byte("guest data"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.noKill = true

	err := m.Stop("web")
	if err == nil {
		t.Fatal("stop must fail when the process is still there")
	}
	if got := statusOf(t, m, "web"); got != StatusRunning {
		t.Fatalf("status = %s; a failed stop must leave the VM as it was", got)
	}
	if data, _ := os.ReadFile(disk); string(data) != "guest data" {
		t.Fatalf("disk touched: %q", data)
	}
	if len(h.synced) != 0 || h.ranCommand("ip link del") {
		t.Fatal("nothing may be synced or torn down under a live process")
	}

	if err := m.Delete("web", true); err == nil {
		t.Fatal("delete --force must fail too, rather than remove a disk a live process holds")
	}
	if !exists(t, disk) {
		t.Fatal("disk removed under a live process")
	}
}

// --- block images after a crash (finding: dirty image mirrored over host) ---

// withBlockMount records a block mount in a VM's runtime state, as if it had
// been created with one.
func withBlockMount(t *testing.T, m *Manager, id, host string) string {
	t.Helper()
	img := filepath.Join(m.cfg.StateDir, "vms", id, "mount-0.ext4")
	if err := os.WriteFile(img, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := LoadState(m.cfg.StateDir, id)
	if err != nil {
		t.Fatal(err)
	}
	st.Mounts = []MountState{{Host: host, Guest: "/data", Method: config.MountBlock, Device: img}}
	if err := SaveState(m.cfg.StateDir, st); err != nil {
		t.Fatal(err)
	}
	return img
}

func TestStopOfACrashedVMOnlySyncsAnImageThatChecksClean(t *testing.T) {
	m, h := newTestManager(t)
	host := t.TempDir()
	mustStart(t, m, "web")
	img := withBlockMount(t, m, "web", host)
	h.crash("web")

	h.checkErr = errors.New("e2fsck: errors found")
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	if len(h.synced) != 0 {
		t.Fatalf("a dirty image was mirrored over the host directory: %v", h.synced)
	}
	if !exists(t, img) {
		t.Fatal("the image must be kept so a restart still has the guest's writes")
	}
	if got := statusOf(t, m, "web"); got != StatusStopped {
		t.Fatalf("status = %s", got)
	}
}

func TestStopOfACrashedVMSyncsACleanImage(t *testing.T) {
	m, h := newTestManager(t)
	host := t.TempDir()
	mustStart(t, m, "web")
	img := withBlockMount(t, m, "web", host)
	h.crash("web")

	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	if len(h.synced) != 1 || h.synced[0] != [2]string{img, host} {
		t.Fatalf("synced = %v", h.synced)
	}
}

func TestStopOfARunningVMSyncsWithoutTheCheck(t *testing.T) {
	m, h := newTestManager(t)
	host := t.TempDir()
	mustStart(t, m, "web")
	withBlockMount(t, m, "web", host)
	h.checkErr = errors.New("would refuse")

	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	if len(h.synced) != 1 || len(h.checked) != 0 {
		t.Fatalf("a VM stopped on purpose is synced as before; synced %v checked %v", h.synced, h.checked)
	}
}

func TestDeleteOfACrashedVMWithADirtyImageWarnsAndStillDeletes(t *testing.T) {
	m, h := newTestManager(t)
	mustStart(t, m, "web")
	withBlockMount(t, m, "web", t.TempDir())
	h.crash("web")
	h.checkErr = errors.New("errors found")

	if err := m.Delete("web", false); err != nil {
		t.Fatal(err)
	}
	if len(h.synced) != 0 {
		t.Fatal("a dirty image was synced")
	}
	if exists(t, filepath.Join(m.cfg.StateDir, "vms", "web")) {
		t.Fatal("delete must still delete")
	}
}

// --- locking (finding: stop/delete raced with start) ---

// blocksWhileLocked runs fn while another process holds the state lock and
// reports whether fn waited for it.
func blocksWhileLocked(t *testing.T, stateDir string, fn func()) bool {
	t.Helper()
	unlock, err := lockState(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
		unlock()
		return false
	case <-time.After(300 * time.Millisecond):
	}
	unlock()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("did not finish after the lock was released")
	}
	return true
}

func TestStopDeleteRestartAndCleanupTakeTheStateLock(t *testing.T) {
	m, _ := newTestManager(t)
	for _, tc := range []struct {
		name string
		fn   func()
	}{
		{"stop", func() { _ = m.Stop("a") }},
		{"delete", func() { _ = m.Delete("a", true) }},
		{"restart", func() { _, _ = m.Restart(context.Background(), "a") }},
		{"cleanup", func() { _ = m.Cleanup(false, "a") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mustStart(t, m, "a")
			if !blocksWhileLocked(t, m.cfg.StateDir, tc.fn) {
				t.Fatalf("%s ran while another fcvm held the state lock", tc.name)
			}
			_ = m.Delete("a", true)
		})
	}
}

// --- the console is not the terminal ---

// Firecracker's stdout carries the guest's serial console. Left pointing at
// the terminal fcvm was started from, the guest's output lands in the user's
// shell, and writes go to a terminal that is gone once fcvm (and sudo's pty)
// exit.
func TestConsoleOutputGoesToALogFileNotTheTerminal(t *testing.T) {
	m, h := newTestManager(t)
	mustStart(t, m, "web")
	want := filepath.Join(m.cfg.StateDir, "vms", "web", "console.log")

	if len(h.consoleFiles) != 1 || h.consoleFiles[0] != want {
		t.Fatalf("Firecracker's stdout was %v, want the VM's console log %s", h.consoleFiles, want)
	}

	// A restart appends: the log of an earlier boot is what you read to see
	// why the VM died.
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	mustStart(t, m, "web")
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "serial: boot 1") || !strings.Contains(string(data), "serial: boot 2") {
		t.Fatalf("console log = %q", data)
	}
}

func TestCreationFailsCleanlyIfTheConsoleLogCannotBeOpened(t *testing.T) {
	m, h := newTestManager(t)
	// A directory where the log file should be.
	vmDir := filepath.Join(m.cfg.StateDir, "vms", "web")
	if err := os.MkdirAll(filepath.Join(vmDir, "console.log"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), "web"); err == nil {
		t.Fatal("expected an error")
	}
	if h.launches != 0 {
		t.Fatal("Firecracker must not be launched without somewhere to send its console")
	}
	if exists(t, vmDir) {
		t.Fatal("a failed creation must not leave the VM behind")
	}
}

// --- the VM must outlive the command that started it ---

// The SDK stops the VM when the context it was started with is cancelled (and
// exec.CommandContext kills it). The command's context is cancelled as soon as
// `fcvm start` returns, so handing it to the SDK races the VM's death against
// the process exiting.
func TestFirecrackerIsNotTiedToTheCommandContext(t *testing.T) {
	m, h := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := m.Start(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	cancel() // what `defer stop()` in Execute does when the command returns

	if len(h.launchCtxs) != 1 || h.launchCtxs[0].Err() != nil {
		t.Fatal("the context given to Firecracker was cancelled with the command's; the SDK would stop the VM")
	}
	if got := statusOf(t, m, "web"); got != StatusRunning {
		t.Fatalf("status = %s", got)
	}
}

func TestInterruptDuringACreationStopsTheVMAndRemovesIt(t *testing.T) {
	m, h := newTestManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	m.ops.waitSSH = func(ctx context.Context, _, _ string, _ time.Duration) error {
		cancel() // Ctrl+C while waiting for the guest
		return ctx.Err()
	}

	_, err := m.Start(ctx, "web")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	proc := h.procs["web"]
	if proc == nil || (&State{PID: proc.Process.Pid}).IsRunning() && waitProcGone(proc.Process.Pid) != nil {
		t.Fatal("the VM was left running after the interrupt")
	}
	if exists(t, filepath.Join(m.cfg.StateDir, "vms", "web")) {
		t.Fatal("an interrupted creation must not leave a VM behind")
	}
}

func TestInterruptDuringARestartLeavesTheVMStoppedWithItsDisk(t *testing.T) {
	m, h := newTestManager(t)
	mustStart(t, m, "web")
	disk := filepath.Join(m.cfg.StateDir, "vms", "web", "rootfs.ext4")
	if err := os.WriteFile(disk, []byte("guest data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.ops.waitSSH = func(ctx context.Context, _, _ string, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	if _, err := m.Start(ctx, "web"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if data, _ := os.ReadFile(disk); string(data) != "guest data" {
		t.Fatalf("disk = %q", data)
	}
	info, err := Inspect(m.cfg.StateDir, "web")
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != StatusStopped || info.StoppedAt.IsZero() {
		t.Fatalf("status %s stoppedAt %v; an interrupted restart is a stop, not a crash", info.Status, info.StoppedAt)
	}
	if info.State != nil {
		t.Fatal("runtime state left behind")
	}
	proc := h.procs["web"]
	if waitProcGone(proc.Process.Pid) != nil {
		t.Fatal("the VM was left running after the interrupt")
	}
}

// waitProcGone waits for a pid to disappear from /proc.
func waitProcGone(pid int) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat("/proc/" + strconv.Itoa(pid)); err != nil {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("still running")
}

// --- review follow-ups ---

func TestFailedRestartOfACrashedVMReadsAsStoppedWithATime(t *testing.T) {
	m, h := newTestManager(t)
	mustStart(t, m, "web")
	disk := filepath.Join(m.cfg.StateDir, "vms", "web", "rootfs.ext4")
	if err := os.WriteFile(disk, []byte("guest data"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.crash("web")
	h.launchErr = errors.New("kvm exploded")

	if _, err := m.Start(context.Background(), "web"); err == nil {
		t.Fatal("expected error")
	}
	info, err := Inspect(m.cfg.StateDir, "web")
	if err != nil {
		t.Fatal(err)
	}
	// The crash's leftovers were torn down, so the VM is now stopped.
	if info.Status != StatusStopped || info.StoppedAt.IsZero() || info.State != nil {
		t.Fatalf("status %s stoppedAt %v state %v", info.Status, info.StoppedAt, info.State)
	}
	if data, _ := os.ReadFile(disk); string(data) != "guest data" {
		t.Fatalf("disk = %q", data)
	}
}

func TestFailedRestartKeepsTheOriginalStopTime(t *testing.T) {
	m, h := newTestManager(t)
	mustStart(t, m, "web")
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	before, _ := Inspect(m.cfg.StateDir, "web")
	h.launchErr = errors.New("kvm exploded")
	if _, err := m.Start(context.Background(), "web"); err == nil {
		t.Fatal("expected error")
	}
	after, _ := Inspect(m.cfg.StateDir, "web")
	if !after.StoppedAt.Equal(before.StoppedAt) {
		t.Fatalf("stop time moved: %v -> %v", before.StoppedAt, after.StoppedAt)
	}
}

// A stop interrupted after the stop record but before state.json is removed
// reads as stopped; stop must finish the job rather than return early.
func TestStopFinishesAnInterruptedStop(t *testing.T) {
	m, h := newTestManager(t)
	st := mustStart(t, m, "web")
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	if err := SaveState(m.cfg.StateDir, &State{ID: "web", TapDev: st.TapDev, GuestSubnet: st.GuestSubnet, HostIface: "eth0"}); err != nil {
		t.Fatal(err)
	}
	before := h.countCommands("ip link del " + st.TapDev)

	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	info, _ := Inspect(m.cfg.StateDir, "web")
	if info.State != nil || info.Status != StatusStopped {
		t.Fatalf("status %s state %v", info.Status, info.State)
	}
	if h.countCommands("ip link del "+st.TapDev) <= before {
		t.Fatal("the leftover TAP device was not released")
	}
}

// A configuration record with no runtime state and no stop record is a
// creation that never finished: the disk may be half built.
func TestACreationThatNeverFinishedIsIncomplete(t *testing.T) {
	m, h := newTestManager(t)
	if err := SaveVMConfig(m.cfg.StateDir, &VMConfig{ID: "half", Config: m.cfg}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.cfg.StateDir, "vms", "half", "rootfs.ext4"), []byte("unpatched"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, m, "half"); got != StatusIncomplete {
		t.Fatalf("status = %s", got)
	}

	if _, err := m.Start(context.Background(), "half"); err == nil || !strings.Contains(err.Error(), "fcvm delete half") {
		t.Fatalf("start must refuse and say how to clear it, got %v", err)
	}
	if _, err := m.Restart(context.Background(), "half"); err == nil || !strings.Contains(err.Error(), "fcvm delete half") {
		t.Fatalf("restart must refuse too, got %v", err)
	}
	if h.launches != 0 {
		t.Fatal("a half-built disk was booted")
	}
	if _, err := RunningState(m.cfg.StateDir, "half"); err == nil || !strings.Contains(err.Error(), "fcvm delete half") {
		t.Fatalf("RunningState: %v", err)
	}

	if err := m.Stop("half"); err != nil { // nothing to stop; it is cleared away
		t.Fatal(err)
	}
	if exists(t, filepath.Join(m.cfg.StateDir, "vms", "half")) {
		t.Fatal("stop of an incomplete VM should clear it")
	}
}

func TestDeleteClearsAnIncompleteVM(t *testing.T) {
	m, _ := newTestManager(t)
	if err := SaveVMConfig(m.cfg.StateDir, &VMConfig{ID: "half", Config: m.cfg}); err != nil {
		t.Fatal(err)
	}
	if err := m.Delete("half", false); err != nil {
		t.Fatal(err)
	}
	if exists(t, filepath.Join(m.cfg.StateDir, "vms", "half")) {
		t.Fatal("not deleted")
	}
}

func TestUnreadableStopRecordIsAnErrorNotACrash(t *testing.T) {
	m, _ := newTestManager(t)
	mustStart(t, m, "web")
	if err := m.Stop("web"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.cfg.StateDir, "vms", "web", stoppedFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(m.cfg.StateDir, "web"); err == nil {
		t.Fatal("a corrupt stop record must not silently read as another status")
	}
}

// New fields on config.Config describe the VM unless they are listed as
// belonging to this host, so forgetting one fails safe: it stays as saved.
func TestShapeFromTreatsUnlistedFieldsAsShape(t *testing.T) {
	saved := config.Default()
	saved.VCPUCount, saved.MemSizeMib = 4, 2048
	saved.KernelArgs, saved.Env = "saved-args", map[string]string{"K": "saved"}
	saved.Mounts = []config.MountConfig{{Host: "/h"}}
	saved.EnableVsock = true
	saved.Jailer.UID, saved.Jailer.PerVMUIDs = 4242, true
	saved.StateDir, saved.Kernel, saved.FirecrackerBin = "/old/state", "/old/vmlinux", "/old/fc"
	saved.Jailer.ChrootBaseDir = "/old/jail"

	cur := config.Default()
	cur.VCPUCount, cur.KernelArgs, cur.EnableVsock = 1, "cur-args", false
	cur.StateDir, cur.Kernel, cur.FirecrackerBin = "/new/state", "/new/vmlinux", "/new/fc"
	cur.Jailer.ChrootBaseDir = "/new/jail"
	cur.StopTimeoutSec = 99

	got := shapeFrom(cur, saved)

	if got.VCPUCount != 4 || got.MemSizeMib != 2048 || got.KernelArgs != "saved-args" || got.Env["K"] != "saved" ||
		len(got.Mounts) != 1 || !got.EnableVsock || got.Jailer.UID != 4242 || !got.Jailer.PerVMUIDs {
		t.Fatalf("shape not kept: %+v", got)
	}
	if got.StateDir != "/new/state" || got.Kernel != "/new/vmlinux" || got.FirecrackerBin != "/new/fc" ||
		got.Jailer.ChrootBaseDir != "/new/jail" || got.StopTimeoutSec != 99 {
		t.Fatalf("host settings not taken from this run: %+v", got)
	}
}
