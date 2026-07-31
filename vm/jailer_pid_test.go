package vm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadJailerPIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "firecracker.pid")
	if err := os.WriteFile(path, []byte("12345\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pid, err := readJailerPIDFile(dir, "/usr/bin/firecracker")
	if err != nil {
		t.Fatal(err)
	}
	if pid != 12345 {
		t.Fatalf("pid = %d, want 12345", pid)
	}
}

func TestReadJailerPIDFileMissing(t *testing.T) {
	_, err := readJailerPIDFile(t.TempDir(), "firecracker")
	if err == nil {
		t.Fatal("expected error for missing pid file")
	}
}

func TestReadJailerPIDFileInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "firecracker.pid")
	if err := os.WriteFile(path, []byte("not-a-pid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readJailerPIDFile(dir, "firecracker"); err == nil {
		t.Fatal("expected error for non-numeric pid")
	}
}

func TestReadJailerPIDFileZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "firecracker.pid")
	if err := os.WriteFile(path, []byte("0"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readJailerPIDFile(dir, "firecracker"); err == nil {
		t.Fatal("expected error for pid 0")
	}
}
