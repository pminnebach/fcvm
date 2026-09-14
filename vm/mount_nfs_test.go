package vm

import (
	"strings"
	"testing"

	"github.com/pminnebach/fcvm/config"
)

// When the CNI plugin chain doesn't publish a gateway, setupMounts must
// refuse an NFS mount with a clear error rather than exporting to the
// guest's own address (which would just make the mount fail silently).
func TestSetupMountsNFSRequiresGatewayForCNI(t *testing.T) {
	cfg := config.Default()
	cfg.StateDir = t.TempDir()
	cfg.Network.CNINetwork = "fcnet"
	m := NewManager(cfg)

	pending := []pendingMount{{
		cfg:       config.MountConfig{Host: "/tmp/whatever"},
		guestPath: "/mnt/whatever",
		slot:      0,
	}}
	_, _, err := m.setupMounts("vm-1", "192.168.127.5", "", pending)
	if err == nil || !strings.Contains(err.Error(), "gateway address") {
		t.Fatalf("err = %v, want mention of missing gateway address", err)
	}
	if err != nil && !strings.Contains(err.Error(), "fcnet") {
		t.Fatalf("err = %v, want it to name the CNI network", err)
	}
}
