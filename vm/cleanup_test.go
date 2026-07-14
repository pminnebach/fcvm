package vm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/fcvm/fcvm/config"
)

func TestCleanupWithoutState(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = dir
	cfg.Jailer.ChrootBaseDir = filepath.Join(dir, "jailer")
	m := NewManager(cfg)

	jailerTree := filepath.Join(cfg.Jailer.ChrootBaseDir, "firecracker", "orphan", "root")
	if err := os.MkdirAll(jailerTree, 0o755); err != nil {
		t.Fatal(err)
	}
	vmDir := filepath.Join(dir, "vms", "orphan")
	if err := os.MkdirAll(vmDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := m.Cleanup(false, "orphan"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.jailerTreeDir("orphan")); !os.IsNotExist(err) {
		t.Fatalf("jailer tree still exists: %v", err)
	}
	if _, err := os.Stat(vmDir); !os.IsNotExist(err) {
		t.Fatalf("vm dir still exists: %v", err)
	}
}

func TestLoadStateMissingIsNotExist(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadState(dir, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}
