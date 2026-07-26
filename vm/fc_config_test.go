package vm

import (
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

	fc, err := buildFirecrackerConfig(cfg, machineBuildInput{
		ID:         "vm-1",
		RootfsPath: "/tmp/rootfs.ext4",
		TapDev:     "fcvm-tap-0",
		TapIP:      "172.16.0.1",
		GuestIP:    "172.16.0.2",
		GuestMAC:   "02:FC:00:00:00:02",
	})
	if err != nil {
		t.Fatal(err)
	}

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

func TestBuildFirecrackerConfigExposeKVM(t *testing.T) {
	cfg := config.Default()
	cfg.Kernel = "/tmp/vmlinux"
	cfg.ExposeKVM = true

	fc, err := buildFirecrackerConfig(cfg, machineBuildInput{
		ID: "vm-1", RootfsPath: "/tmp/r.ext4",
		TapDev: "t", TapIP: "172.16.0.1", GuestIP: "172.16.0.2", GuestMAC: "02:00:00:00:00:01",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := defaultKernelArgs + " pci=on fcvm.kvm=1"
	if fc.KernelArgs != want {
		t.Fatalf("got %q want %q", fc.KernelArgs, want)
	}

	cfg.KernelArgs = defaultKernelArgs + " pci=on fcvm.kvm=1"
	fc2, err := buildFirecrackerConfig(cfg, machineBuildInput{
		ID: "vm-1", RootfsPath: "/tmp/r.ext4",
		TapDev: "t", TapIP: "172.16.0.1", GuestIP: "172.16.0.2", GuestMAC: "02:00:00:00:00:01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if fc2.KernelArgs != cfg.KernelArgs {
		t.Fatalf("double-append: got %q", fc2.KernelArgs)
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

	fc, err := buildFirecrackerConfig(cfg, machineBuildInput{
		ID: "vm-1", RootfsPath: "/tmp/r.ext4",
		BlockImages: []string{"/tmp/data0.ext4"},
		TapDev:      "t", TapIP: "172.16.0.1", GuestIP: "172.16.0.2", GuestMAC: "02:00:00:00:00:01",
	})
	if err != nil {
		t.Fatal(err)
	}
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
	if len(fc.Drives) != 2 || *fc.Drives[1].DriveID != "data0" {
		t.Fatalf("drives = %+v", fc.Drives)
	}
}
