package vm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pminnebach/fcvm/config"
)

func TestJailerTreeDir(t *testing.T) {
	cfg := config.Default()
	cfg.Jailer.ChrootBaseDir = "/home/user/.fcvm/jailer"
	m := NewManager(cfg)
	got := m.jailerTreeDir("testvm")
	want := "/home/user/.fcvm/jailer/firecracker/testvm"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRemoveJailerTree(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Jailer.ChrootBaseDir = dir
	m := NewManager(cfg)
	tree := filepath.Join(m.jailerTreeDir("testvm"), "root", "dev", "net", "tun")
	if err := os.MkdirAll(filepath.Dir(tree), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tree, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.removeJailerTree("testvm")
	if _, err := os.Stat(m.jailerTreeDir("testvm")); !os.IsNotExist(err) {
		t.Fatalf("jailer tree still exists: %v", err)
	}
}
