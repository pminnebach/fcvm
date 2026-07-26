package network

import (
	"context"
	"testing"
)

func TestTeardownCNIEmptyNoop(t *testing.T) {
	if err := TeardownCNI(context.Background(), "", "fcnet"); err != nil {
		t.Fatal(err)
	}
	if err := TeardownCNI(context.Background(), "vm-1", ""); err != nil {
		t.Fatal(err)
	}
}

func TestNetNSPath(t *testing.T) {
	if got := NetNSPath("vm-1"); got != "/var/run/netns/vm-1" {
		t.Fatalf("got %q", got)
	}
}
