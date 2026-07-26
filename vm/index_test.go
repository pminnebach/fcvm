package vm

import (
	"os"
	"testing"

	"github.com/pminnebach/fcvm/config"
)

// Allocation must pick the lowest free index. Deriving it from the number of
// VMs reuses the index of any stopped VM, and SetupTap would then delete the
// TAP device of the VM still holding that index.
func TestNextVMIndexPicksLowestFree(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = dir
	m := NewManager(cfg)

	save := func(id string, index int) {
		t.Helper()
		if err := SaveState(dir, &State{ID: id, Index: index, TapDev: "fcvm-tap-" + itoa(index)}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := m.nextVMIndex()
	if err != nil || got != 0 {
		t.Fatalf("empty state dir: got %d (%v), want 0", got, err)
	}

	save("a", 0)
	save("c", 2)
	got, err = m.nextVMIndex()
	if err != nil || got != 1 {
		t.Fatalf("gap at 1: got %d (%v), want 1", got, err)
	}

	save("b", 1)
	got, err = m.nextVMIndex()
	if err != nil || got != 3 {
		t.Fatalf("no gap: got %d (%v), want 3", got, err)
	}

	// Stopping the first VM must not free an index still used by another VM.
	if err := RemoveState(dir, "a"); err != nil {
		t.Fatal(err)
	}
	got, err = m.nextVMIndex()
	if err != nil || got != 0 {
		t.Fatalf("after removing index 0: got %d (%v), want 0", got, err)
	}
}

// States written before the index field existed still claim their slot via
// the recorded TAP device name.
func TestClaimedIndicesFromLegacyTapDev(t *testing.T) {
	claimed := ClaimedIndices([]State{
		{ID: "legacy", TapDev: "fcvm-tap-4"},
		{ID: "cni", NetworkMode: NetworkModeCNI, Index: 2},
	})
	if !claimed[4] {
		t.Fatal("legacy TAP state did not claim index 4")
	}
	if !claimed[2] {
		t.Fatal("CNI state did not claim its index")
	}
}

func TestLockStateIsExclusive(t *testing.T) {
	dir := t.TempDir()
	unlock, err := lockState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir + "/.lock"); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
	unlock()
	// A second acquisition must succeed once released.
	unlock2, err := lockState(dir)
	if err != nil {
		t.Fatalf("re-acquire after unlock: %v", err)
	}
	unlock2()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
