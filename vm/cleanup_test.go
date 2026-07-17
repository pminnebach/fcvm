package vm

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pminnebach/fcvm/config"
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
	rootfs := filepath.Join(vmDir, "rootfs.ext4")
	if err := os.WriteFile(rootfs, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	m.cleanupVM("orphan", nil)

	if _, err := os.Stat(m.jailerTreeDir("orphan")); !os.IsNotExist(err) {
		t.Fatalf("jailer tree still exists: %v", err)
	}
	if _, err := os.Stat(rootfs); !os.IsNotExist(err) {
		t.Fatalf("rootfs still exists: %v", err)
	}
	if _, err := os.Stat(vmDir); !os.IsNotExist(err) {
		t.Fatalf("vm dir still exists: %v", err)
	}
}

func TestCleanupAllRemovesOrphans(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	dir := t.TempDir()
	cfg := config.Default()
	cfg.StateDir = dir
	cfg.Jailer.ChrootBaseDir = filepath.Join(dir, "jailer")
	m := NewManager(cfg)

	statedDir := filepath.Join(dir, "vms", "stated")
	if err := os.MkdirAll(statedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(statedDir, "rootfs.ext4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := &State{ID: "stated", TapDev: "tap-fcvm0"}
	if err := SaveState(dir, state); err != nil {
		t.Fatal(err)
	}

	orphanDir := filepath.Join(dir, "vms", "orphan")
	if err := os.MkdirAll(orphanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphanDir, "rootfs.ext4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := m.Cleanup(true, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statedDir); !os.IsNotExist(err) {
		t.Fatalf("stated vm dir still exists: %v", err)
	}
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Fatalf("orphan vm dir still exists: %v", err)
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
