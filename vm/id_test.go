package vm

import (
	"os"
	"path/filepath"
	"testing"
)

// A VM id reaches os.RemoveAll as root. filepath.Join cleans "..", so an
// unvalidated id escapes the state directory entirely.
func TestRemoveStateRefusesTraversal(t *testing.T) {
	base := t.TempDir()
	stateDir := filepath.Join(base, "state")
	victim := filepath.Join(base, "victim")
	if err := os.MkdirAll(filepath.Join(stateDir, "vms"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := RemoveState(stateDir, "../../victim"); err == nil {
		t.Fatal("RemoveState accepted an id that escapes the state dir")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("victim directory was deleted: %v", err)
	}
}

func TestValidateID(t *testing.T) {
	valid := []string{"vm-1", "my_vm", "a.b", "vm-1784063753", "A0"}
	for _, id := range valid {
		if err := ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) = %v, want nil", id, err)
		}
	}
	invalid := []string{"", ".", "..", "../x", "a/b", `a\b`, "a b", "a;b", "a$b", string(make([]byte, 65))}
	for _, id := range invalid {
		if err := ValidateID(id); err == nil {
			t.Errorf("ValidateID(%q) = nil, want an error", id)
		}
	}
}

func TestLoadStateRejectsTraversal(t *testing.T) {
	if _, err := LoadState(t.TempDir(), "../escape"); err == nil {
		t.Fatal("LoadState accepted a traversing id")
	}
}
