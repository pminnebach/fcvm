package vm

import (
	"path/filepath"
	"testing"
)

func TestJailerHostLogPath(t *testing.T) {
	chroot := "/home/user/.fcvm/jailer/firecracker/testvm/root"
	got := filepath.Join(chroot, "firecracker.log")
	want := "/home/user/.fcvm/jailer/firecracker/testvm/root/firecracker.log"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
