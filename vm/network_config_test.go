package vm

import (
	"strings"
	"testing"

	"github.com/pminnebach/fcvm/config"
)

// tapConfig builds the real Firecracker config for a static TAP VM, so these
// assertions fail if buildFirecrackerConfig regresses rather than only if the
// SDK does.
func tapConfig(t *testing.T) (cfg config.Config, guestIP, tapIP string) {
	t.Helper()
	cfg = config.Default()
	cfg.Kernel = "/tmp/vmlinux"
	return cfg, "172.16.0.2", "172.16.0.1"
}

func TestNetworkConfigValidatePassesWithoutDuplicateIP(t *testing.T) {
	cfg, guestIP, tapIP := tapConfig(t)
	fc := buildFirecrackerConfig(cfg, machineBuildInput{
		ID: "vm-1", RootfsPath: "/tmp/r.ext4",
		TapDev: "fcvm-tap-0", TapIP: tapIP, GuestIP: guestIP, GuestMAC: "06:00:AC:10:00:02",
		JailerUID: 1000, JailerGID: 1000,
	})
	if err := fc.ValidateNetwork(); err != nil {
		t.Fatalf("ValidateNetwork: %v", err)
	}
	if strings.Contains(fc.KernelArgs, "ip=") {
		t.Fatalf("kernel args should not include ip= before SDK setup: %q", fc.KernelArgs)
	}
}

func TestNetworkConfigValidateRejectsDuplicateIP(t *testing.T) {
	cfg, guestIP, tapIP := tapConfig(t)
	cfg.KernelArgs = "console=ttyS0 reboot=k panic=1 ip=172.16.0.2::172.16.0.1:255.255.255.252::eth0:off"
	fc := buildFirecrackerConfig(cfg, machineBuildInput{
		ID: "vm-1", RootfsPath: "/tmp/r.ext4",
		TapDev: "fcvm-tap-0", TapIP: tapIP, GuestIP: guestIP, GuestMAC: "06:00:AC:10:00:02",
		JailerUID: 1000, JailerGID: 1000,
	})
	if err := fc.ValidateNetwork(); err == nil {
		t.Fatal("expected ValidateNetwork error for duplicate ip= + IPConfiguration")
	}
}

// The guest must inherit the configured resolvers, not a hardcoded public one.
func TestNetworkConfigUsesConfiguredNameservers(t *testing.T) {
	cfg, guestIP, tapIP := tapConfig(t)
	cfg.Network.Nameservers = []string{"10.0.0.53"}
	fc := buildFirecrackerConfig(cfg, machineBuildInput{
		ID: "vm-1", RootfsPath: "/tmp/r.ext4",
		TapDev: "fcvm-tap-0", TapIP: tapIP, GuestIP: guestIP, GuestMAC: "06:00:AC:10:00:02",
		JailerUID: 1000, JailerGID: 1000,
	})
	ns := fc.NetworkInterfaces[0].StaticConfiguration.IPConfiguration.Nameservers
	if len(ns) != 1 || ns[0] != "10.0.0.53" {
		t.Fatalf("nameservers = %v, want [10.0.0.53]", ns)
	}
}
