package network

import "testing"

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
