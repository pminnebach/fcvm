package network

import (
	"fmt"
	"testing"
)

func TestGuestMAC(t *testing.T) {
	mac := GuestMAC("172.16.0.2")
	want := "06:00:AC:10:00:02"
	if mac != want {
		t.Fatalf("GuestMAC() = %q, want %q", mac, want)
	}
}

func TestSubnetForIndex(t *testing.T) {
	tap, guest := SubnetForIndex("172.16.0.1", "172.16.0.2", 1)
	if tap != "172.16.1.1" || guest != "172.16.1.2" {
		t.Fatalf("SubnetForIndex(1) = %s/%s, want 172.16.1.1/172.16.1.2", tap, guest)
	}
}

func TestTapDevNameWithinLinuxLimit(t *testing.T) {
	for _, index := range []int{0, 1, 42, 999999} {
		name := TapDevName(index)
		if len(name) > maxIfNameLen {
			t.Fatalf("TapDevName(%d) = %q len %d exceeds Linux limit %d", index, name, len(name), maxIfNameLen)
		}
	}
	// ponytail: default VM id vm-<unix_ts> made fcvm-tap-vm-<ts> (22 chars) fail tuntap add
	long := fmt.Sprintf("fcvm-tap-%s", "vm-1784063753")
	if len(long) <= maxIfNameLen {
		t.Fatalf("sanity: old naming should exceed limit, got len %d", len(long))
	}
}
