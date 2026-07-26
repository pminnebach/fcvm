package network

import (
	"errors"
	"fmt"
	"strings"
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
	tap, guest, err := SubnetForIndex("172.16.0.1", "172.16.0.2", 1)
	if err != nil {
		t.Fatal(err)
	}
	if tap != "172.16.1.1" || guest != "172.16.1.2" {
		t.Fatalf("SubnetForIndex(1) = %s/%s, want 172.16.1.1/172.16.1.2", tap, guest)
	}
}

// Wrapping the third octet would hand two VMs the same address, so an index
// past the addressable range must be an error, not a silent collision.
func TestSubnetForIndexRejectsOverflow(t *testing.T) {
	if _, _, err := SubnetForIndex("172.16.0.1", "172.16.0.2", 256); err == nil {
		t.Fatal("expected an error for an index past the addressable range")
	}
	// Non-zero base: the range that remains is smaller.
	if _, _, err := SubnetForIndex("172.16.200.1", "172.16.200.2", 100); err == nil {
		t.Fatal("expected an error when the base octet leaves less room than the index")
	}
	if _, _, err := SubnetForIndex("172.16.0.1", "172.16.0.2", -1); err == nil {
		t.Fatal("expected an error for a negative index")
	}
}

func TestIndexFromTapDev(t *testing.T) {
	for i := range 5 {
		got, ok := IndexFromTapDev(TapDevName(i))
		if !ok || got != i {
			t.Fatalf("round trip index %d: got %d, ok=%v", i, got, ok)
		}
	}
	if _, ok := IndexFromTapDev("eth0"); ok {
		t.Fatal("eth0 is not an fcvm tap device")
	}
	if _, ok := IndexFromTapDev(""); ok {
		t.Fatal("empty device name is not an fcvm tap device")
	}
}

// Setup and teardown must produce mirrored argv, or teardown leaves orphan
// rules on the host.
func TestVMRulesAddDeleteMirror(t *testing.T) {
	rules := vmRules("fcvm-tap-0", "172.16.0.0/30", "eth0")
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	for _, r := range rules {
		add := r.args("-A")
		del := r.args("-D")
		if len(add) != len(del) {
			t.Fatalf("add/delete arity differs: %v vs %v", add, del)
		}
		for i := range add {
			if i == len(add)-len(r.spec)-2 {
				continue // the operation flag itself
			}
			if add[i] != del[i] {
				t.Fatalf("add/delete differ at %d: %v vs %v", i, add, del)
			}
		}
	}
	// The nat rule must place -t nat before the operation flag, or iptables
	// consumes "-t" as the chain name.
	nat := rules[2].args("-A")
	if nat[0] != "-t" || nat[1] != "nat" || nat[2] != "-A" || nat[3] != "POSTROUTING" {
		t.Fatalf("nat argv malformed: %v", nat)
	}
}

// fcvm must never change the host's FORWARD policy: on a host that set DROP
// deliberately, ACCEPT is a silent downgrade that outlives the VM.
func TestSetupTapNeverSetsChainPolicy(t *testing.T) {
	var calls [][]string
	restore := SetRunner(func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		if name == "ip" && len(args) > 1 && args[0] == "link" && args[1] == "show" {
			return errShouldNotExist
		}
		return nil
	})
	defer restore()

	if err := SetupTap("fcvm-tap-0", "172.16.0.1", "172.16.0.2", "eth0"); err != nil {
		t.Fatal(err)
	}
	for _, c := range calls {
		joined := fmt.Sprint(c)
		if strings.Contains(joined, "-P") {
			t.Fatalf("SetupTap changed a chain policy: %v", c)
		}
		if strings.Contains(joined, "ip_forward") {
			t.Fatalf("SetupTap wrote ip_forward directly: %v", c)
		}
	}
}

var errShouldNotExist = errors.New("Cannot find device")

func TestSetupTapRefusesExistingDevice(t *testing.T) {
	restore := SetRunner(func(name string, args ...string) error { return nil })
	defer restore()
	err := SetupTap("fcvm-tap-0", "172.16.0.1", "172.16.0.2", "eth0")
	if err == nil {
		t.Fatal("expected SetupTap to refuse a device that already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseDefaultIface(t *testing.T) {
	dev, err := parseDefaultIface([]byte(`[{"dst":"default","gateway":"10.0.0.1","dev":"enp0s3"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if dev != "enp0s3" {
		t.Fatalf("dev = %q, want enp0s3", dev)
	}
	if _, err := parseDefaultIface([]byte(`[]`)); err == nil {
		t.Fatal("no default route should be an error, not a guess")
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
