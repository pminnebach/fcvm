package vm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListVMDirIDs(t *testing.T) {
	dir := t.TempDir()

	ids, err := ListVMDirIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("missing vms dir: got %v", ids)
	}

	vmsDir := filepath.Join(dir, "vms")
	if err := os.MkdirAll(vmsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ids, err = ListVMDirIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("empty vms dir: got %v", ids)
	}

	for _, id := range []string{"with-state", "orphan"} {
		if err := os.MkdirAll(filepath.Join(vmsDir, id), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	state := &State{ID: "with-state"}
	if err := SaveState(dir, state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vmsDir, "orphan", "rootfs.ext4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ids, err = ListVMDirIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d ids, want 2: %v", len(ids), ids)
	}
}
