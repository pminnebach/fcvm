package vm

import (
	"os"
	"testing"

	"github.com/fcvm/fcvm/config"
)

func TestChownForJailer(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	dir := t.TempDir()
	path := dir + "/drive.ext4"
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Jailer.UID = 65534 // nobody
	cfg.Jailer.GID = 65534
	m := NewManager(cfg)
	if err := m.chownForJailer(path); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Sys() == nil {
		t.Skip("cannot inspect ownership on this platform")
	}
}
