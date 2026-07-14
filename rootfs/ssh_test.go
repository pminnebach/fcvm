package rootfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInjectSSHKey(t *testing.T) {
	dir := t.TempDir()
	pub := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITest fcvm@host"
	if err := InjectSSHKey(dir, pub); err != nil {
		t.Fatal(err)
	}
	authKeys := filepath.Join(dir, "root/.ssh/authorized_keys")
	data, err := os.ReadFile(authKeys)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != pub+"\n" {
		t.Fatalf("authorized_keys = %q", string(data))
	}
	st, err := os.Stat(authKeys)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("authorized_keys mode = %o", st.Mode().Perm())
	}
	cfg := filepath.Join(dir, "etc/ssh/sshd_config.d/fcvm.conf")
	if _, err := os.Stat(cfg); err != nil {
		t.Fatalf("missing sshd drop-in: %v", err)
	}
}
