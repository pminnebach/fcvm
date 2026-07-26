package vm

import (
	"os"
	"testing"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"

	"github.com/pminnebach/fcvm/config"
)

func TestBuildFirecrackerConfigDefaults(t *testing.T) {
	cfg := config.Default()
	cfg.Kernel = "/tmp/vmlinux"
	cfg.FirecrackerBin = "/tmp/firecracker"
	cfg.JailerBin = "/tmp/jailer"
	cfg.Jailer.ChrootBaseDir = "/tmp/jailer-base"
	cfg.VCPUCount = 2
	cfg.MemSizeMib = 512

	fc := buildFirecrackerConfig(cfg, machineBuildInput{
		ID:         "vm-1",
		RootfsPath: "/tmp/rootfs.ext4",
		TapDev:     "fcvm-tap-0",
		TapIP:      "172.16.0.1",
		GuestIP:    "172.16.0.2",
		GuestMAC:   "02:FC:00:00:00:02",
		JailerUID:  1000,
		JailerGID:  1000,
	})

	if fc.KernelArgs != defaultKernelArgs {
		t.Fatalf("KernelArgs = %q, want %q", fc.KernelArgs, defaultKernelArgs)
	}
	if fc.LogLevel != "Info" {
		t.Fatalf("LogLevel = %q, want Info", fc.LogLevel)
	}
	if fc.SocketPath != fcSocketName || fc.LogPath != fcLogName {
		t.Fatalf("paths = %q/%q", fc.SocketPath, fc.LogPath)
	}
	if fc.MmdsVersion == "" {
		t.Fatal("MmdsVersion unset")
	}
	if len(fc.NetworkInterfaces) != 1 || !fc.NetworkInterfaces[0].AllowMMDS {
		t.Fatal("expected single NIC with AllowMMDS")
	}
	if fc.NetworkInterfaces[0].StaticConfiguration == nil {
		t.Fatal("expected StaticConfiguration")
	}
	if len(fc.Drives) != 1 || *fc.Drives[0].DriveID != "rootfs" {
		t.Fatalf("drives = %+v", fc.Drives)
	}
	if fc.JailerCfg == nil {
		t.Fatal("JailerCfg nil")
	}
	if fc.JailerCfg.NumaNode == nil || *fc.JailerCfg.NumaNode != 0 {
		t.Fatalf("NumaNode = %v, want 0", fc.JailerCfg.NumaNode)
	}
	if fc.JailerCfg.Daemonize {
		t.Fatal("Daemonize should be false by default")
	}
	if fc.MachineCfg.Smt != nil {
		t.Fatal("Smt should be unset by default")
	}
	if fc.MachineCfg.CPUTemplate != "" {
		t.Fatalf("CPUTemplate = %q, want empty", fc.MachineCfg.CPUTemplate)
	}
}

func TestBuildFirecrackerConfigOverrides(t *testing.T) {
	cfg := config.Default()
	cfg.Kernel = "/tmp/vmlinux"
	cfg.KernelArgs = "console=ttyS0 custom=1"
	cfg.LogLevel = "Debug"
	cfg.CPUTemplate = "T2"
	cfg.DisableSMT = true
	cfg.Jailer.NumaNode = 1
	cfg.Jailer.Daemonize = true
	cfg.Jailer.ParentCgroup = "fcvm"
	cfg.Jailer.Cgroup = []string{"memory.max=1G"}

	fc := buildFirecrackerConfig(cfg, machineBuildInput{
		ID: "vm-1", RootfsPath: "/tmp/r.ext4",
		BlockDrives: []blockDrive{{Path: "/tmp/data0.ext4"}, {Path: "/tmp/data1.ext4", ReadOnly: true}},
		TapDev:      "t", TapIP: "172.16.0.1", GuestIP: "172.16.0.2", GuestMAC: "02:00:00:00:00:01",
		JailerUID: 1000, JailerGID: 1000,
	})
	if fc.KernelArgs != "console=ttyS0 custom=1" {
		t.Fatalf("KernelArgs = %q", fc.KernelArgs)
	}
	if fc.LogLevel != "Debug" {
		t.Fatalf("LogLevel = %q", fc.LogLevel)
	}
	if fc.MachineCfg.CPUTemplate != models.CPUTemplateT2 {
		t.Fatalf("CPUTemplate = %q", fc.MachineCfg.CPUTemplate)
	}
	if fc.MachineCfg.Smt == nil || *fc.MachineCfg.Smt {
		t.Fatalf("Smt = %v, want false", fc.MachineCfg.Smt)
	}
	if *fc.JailerCfg.NumaNode != 1 {
		t.Fatalf("NumaNode = %d", *fc.JailerCfg.NumaNode)
	}
	if !fc.JailerCfg.Daemonize {
		t.Fatal("Daemonize want true")
	}
	if fc.JailerCfg.ParentCgroup != "fcvm" {
		t.Fatalf("ParentCgroup = %q", fc.JailerCfg.ParentCgroup)
	}
	if len(fc.JailerCfg.CgroupArgs) != 1 || fc.JailerCfg.CgroupArgs[0] != "memory.max=1G" {
		t.Fatalf("CgroupArgs = %v", fc.JailerCfg.CgroupArgs)
	}
	if len(fc.Drives) != 3 || *fc.Drives[1].DriveID != "data0" {
		t.Fatalf("drives = %+v", fc.Drives)
	}
	// A read-only mount must not be attached writable.
	if *fc.Drives[1].IsReadOnly {
		t.Fatal("data0 should be writable")
	}
	if !*fc.Drives[2].IsReadOnly {
		t.Fatal("data1 was requested read-only but is attached writable")
	}
}

func TestBuildFirecrackerConfigCNI(t *testing.T) {
	cfg := config.Default()
	cfg.Kernel = "/tmp/vmlinux"
	cfg.Network.CNINetwork = "fcnet"

	fc := buildFirecrackerConfig(cfg, machineBuildInput{
		ID: "vm-1", RootfsPath: "/tmp/r.ext4",
		JailerUID: 1000, JailerGID: 1000,
	})
	iface := fc.NetworkInterfaces[0]
	if iface.StaticConfiguration != nil {
		t.Fatal("expected no StaticConfiguration for CNI")
	}
	if iface.CNIConfiguration == nil || iface.CNIConfiguration.NetworkName != "fcnet" {
		t.Fatalf("CNIConfiguration = %+v", iface.CNIConfiguration)
	}
	if iface.CNIConfiguration.IfName != "veth0" || iface.CNIConfiguration.VMIfName != "eth0" {
		t.Fatalf("CNI ifnames = %+v", iface.CNIConfiguration)
	}
}

func TestJailerCredsPerVM(t *testing.T) {
	cfg := config.Default()
	cfg.Jailer.UID = 1000
	cfg.Jailer.GID = 1000
	uid, gid := jailerCreds(cfg, 3)
	if uid != 1000 || gid != 1000 {
		t.Fatalf("shared: got %d/%d", uid, gid)
	}
	cfg.Jailer.PerVMUIDs = true
	uid, gid = jailerCreds(cfg, 3)
	if uid != 1003 || gid != 1003 {
		t.Fatalf("per-vm: got %d/%d", uid, gid)
	}

	fc := buildFirecrackerConfig(cfg, machineBuildInput{
		ID: "vm-1", RootfsPath: "/tmp/r.ext4",
		TapDev: "t", TapIP: "172.16.0.1", GuestIP: "172.16.0.2", GuestMAC: "02:00:00:00:00:01",
		JailerUID: uid, JailerGID: gid,
	})
	if *fc.JailerCfg.UID != 1003 || *fc.JailerCfg.GID != 1003 {
		t.Fatalf("JailerCfg uid/gid = %d/%d", *fc.JailerCfg.UID, *fc.JailerCfg.GID)
	}
}

func TestTeardownStateSkipsTapForCNI(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = dir
	cfg.Jailer.ChrootBaseDir = dir + "/jailer"
	m := NewManager(cfg)

	// CNI state with empty TapDev must not panic; TeardownTap is not called.
	state := &State{
		ID:          "cni-vm",
		NetworkMode: NetworkModeCNI,
		CNINetwork:  "fcnet",
		ChrootDir:   dir + "/jailer/firecracker/cni-vm/root",
	}
	if err := os.MkdirAll(state.ChrootDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m.teardownState(state)
}

func TestStateIsCNI(t *testing.T) {
	if (&State{NetworkMode: NetworkModeCNI}).IsCNI() != true {
		t.Fatal("expected CNI")
	}
	if (&State{NetworkMode: NetworkModeTAP, TapDev: "x"}).IsCNI() {
		t.Fatal("tap should not be CNI")
	}
}
