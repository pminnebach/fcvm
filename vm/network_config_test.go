package vm

import (
	"net"
	"strings"
	"testing"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
)

func fcvmNetworkConfig(guestIP, tapIP string) firecracker.Config {
	return firecracker.Config{
		KernelArgs: "console=ttyS0 reboot=k panic=1 pci=on",
		NetworkInterfaces: []firecracker.NetworkInterface{{
			StaticConfiguration: &firecracker.StaticNetworkConfiguration{
				MacAddress:  "06:00:AC:10:00:02",
				HostDevName: "fcvm-tap-test",
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
		}},
	}
}

func TestNetworkConfigValidatePassesWithoutDuplicateIP(t *testing.T) {
	cfg := fcvmNetworkConfig("172.16.0.2", "172.16.0.1")
	if err := cfg.ValidateNetwork(); err != nil {
		t.Fatalf("ValidateNetwork: %v", err)
	}
	if strings.Contains(cfg.KernelArgs, "ip=") {
		t.Fatalf("kernel args should not include ip= before SDK setup: %q", cfg.KernelArgs)
	}
}

func TestNetworkConfigValidateRejectsDuplicateIP(t *testing.T) {
	cfg := fcvmNetworkConfig("172.16.0.2", "172.16.0.1")
	cfg.KernelArgs = "console=ttyS0 reboot=k panic=1 pci=on ip=172.16.0.2::172.16.0.1:255.255.255.252::eth0:off"
	err := cfg.ValidateNetwork()
	if err == nil {
		t.Fatal("expected ValidateNetwork error for duplicate ip= + IPConfiguration")
	}
}
