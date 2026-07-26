package vm

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not installed", name)
	}
}

// The whole point of a block-fallback mount is that the guest's writes reach
// the host. Before write-back existed, stop discarded the image and the writes
// with it.
func TestBlockMountRoundTripsToHost(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("loop mounting requires root")
	}
	requireTool(t, "mkfs.ext4")
	requireTool(t, "truncate")

	base := t.TempDir()
	host := filepath.Join(base, "host")
	if err := os.MkdirAll(filepath.Join(host, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(host, "keep.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(host, "sub", "gone.txt"), []byte("delete me"), 0o644); err != nil {
		t.Fatal(err)
	}

	img := filepath.Join(base, "mount-0.ext4")
	if err := syncDirToExt4(host, img, ""); err != nil {
		t.Fatalf("build image: %v", err)
	}

	// Stand in for the guest writing to the mounted image.
	mnt := filepath.Join(base, "mnt")
	if err := os.Mkdir(mnt, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("mount", "-o", "loop", img, mnt).CombinedOutput(); err != nil {
		t.Skipf("loop mount unavailable: %s: %v", out, err)
	}
	if err := os.WriteFile(filepath.Join(mnt, "created-in-guest.txt"), []byte("from guest"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mnt, "keep.txt"), []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(mnt, "sub", "gone.txt")); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("umount", mnt).CombinedOutput(); err != nil {
		t.Fatalf("umount: %s: %v", out, err)
	}

	if err := syncExt4ToDir(img, host); err != nil {
		t.Fatalf("write back: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(host, "created-in-guest.txt"))
	if err != nil || string(got) != "from guest" {
		t.Fatalf("new guest file did not reach the host: %q %v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(host, "keep.txt"))
	if err != nil || string(got) != "modified" {
		t.Fatalf("modified file = %q %v, want \"modified\"", got, err)
	}
	if _, err := os.Stat(filepath.Join(host, "sub", "gone.txt")); !os.IsNotExist(err) {
		t.Fatal("file deleted in the guest is still on the host")
	}
	if _, err := os.Stat(filepath.Join(host, "lost+found")); err == nil {
		t.Fatal("ext4 lost+found leaked into the host directory")
	}
}

func TestMirrorDirPrunesAndCopies(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "src")
	dst := filepath.Join(base, "dst")
	for _, d := range []string{src, dst, filepath.Join(src, "keep"), filepath.Join(dst, "obsolete")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "keep", "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "obsolete", "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "lost+found"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := mirrorDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "obsolete")); !os.IsNotExist(err) {
		t.Fatal("stale directory was not pruned")
	}
	if got, err := os.ReadFile(filepath.Join(dst, "keep", "a.txt")); err != nil || string(got) != "a" {
		t.Fatalf("new file = %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "lost+found")); err == nil {
		t.Fatal("lost+found should not be mirrored")
	}
}
