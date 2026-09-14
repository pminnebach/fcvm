package vm

import (
	"net"
	"strings"
	"testing"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
)

func machineWithIface(iface firecracker.NetworkInterface) *firecracker.Machine {
	return &firecracker.Machine{Cfg: firecracker.Config{
		NetworkInterfaces: []firecracker.NetworkInterface{iface},
	}}
}

func TestResolveCNIAddrsHappyPath(t *testing.T) {
	m := machineWithIface(firecracker.NetworkInterface{
		StaticConfiguration: &firecracker.StaticNetworkConfiguration{
			MacAddress: "02:FC:00:00:00:05",
			IPConfiguration: &firecracker.IPConfiguration{
				IPAddr:  net.IPNet{IP: net.ParseIP("192.168.127.5")},
				Gateway: net.ParseIP("192.168.127.1"),
			},
		},
	})
	ip, gw, mac, err := resolveCNIAddrs(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "192.168.127.5" || gw != "192.168.127.1" || mac != "02:FC:00:00:00:05" {
		t.Fatalf("ip=%q gw=%q mac=%q", ip, gw, mac)
	}
}

func TestResolveCNIAddrsNoGateway(t *testing.T) {
	// Some IPAM configs never publish a gateway; resolveCNIAddrs must not
	// error out on that alone (the empty-gateway case is rejected later, by
	// setupMounts, only when an NFS mount actually needs it).
	m := machineWithIface(firecracker.NetworkInterface{
		StaticConfiguration: &firecracker.StaticNetworkConfiguration{
			MacAddress: "02:FC:00:00:00:05",
			IPConfiguration: &firecracker.IPConfiguration{
				IPAddr: net.IPNet{IP: net.ParseIP("192.168.127.5")},
			},
		},
	})
	ip, gw, _, err := resolveCNIAddrs(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "192.168.127.5" || gw != "" {
		t.Fatalf("ip=%q gw=%q, want gw empty", ip, gw)
	}
}

func TestResolveCNIAddrsNoInterfaces(t *testing.T) {
	m := &firecracker.Machine{Cfg: firecracker.Config{}}
	if _, _, _, err := resolveCNIAddrs(m); err == nil || !strings.Contains(err.Error(), "no network interfaces") {
		t.Fatalf("err = %v, want mention of no network interfaces", err)
	}
}

func TestResolveCNIAddrsMissingStaticConfiguration(t *testing.T) {
	// StaticConfiguration stays nil when the CNI result had no plugin that
	// filled it in — i.e. tc-redirect-tap is missing from the conflist.
	m := machineWithIface(firecracker.NetworkInterface{})
	_, _, _, err := resolveCNIAddrs(m)
	if err == nil || !strings.Contains(err.Error(), "tc-redirect-tap") {
		t.Fatalf("err = %v, want mention of tc-redirect-tap", err)
	}
}

func TestResolveCNIAddrsNoIPConfiguration(t *testing.T) {
	m := machineWithIface(firecracker.NetworkInterface{
		StaticConfiguration: &firecracker.StaticNetworkConfiguration{MacAddress: "02:FC:00:00:00:05"},
	})
	_, _, _, err := resolveCNIAddrs(m)
	if err == nil || !strings.Contains(err.Error(), "no guest IP") {
		t.Fatalf("err = %v, want mention of no guest IP", err)
	}
}
