package network

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNFSExportLineMapsToOwner(t *testing.T) {
	line := nfsExportLine("/tmp/fcvm-exports/test-0/share", 501, 20, false)
	if runtime.GOOS == "darwin" {
		if !strings.Contains(line, "-mapall=501") {
			t.Fatalf("darwin export = %q", line)
		}
		return
	}
	if !strings.Contains(line, "all_squash,anonuid=501,anongid=20") {
		t.Fatalf("linux export = %q", line)
	}
	if strings.Contains(line, "no_root_squash") {
		t.Fatal("must not use no_root_squash")
	}
}

func TestPathOwnerIDs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	uid, gid, err := pathOwnerIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if uid < 0 || gid < 0 {
		t.Fatalf("uid=%d gid=%d", uid, gid)
	}
}

func TestIsMountPointFalseForTempDir(t *testing.T) {
	dir := t.TempDir()
	if isMountPoint(dir) {
		t.Fatalf("temp dir should not be a mount point: %s", dir)
	}
}

func TestRemoveExportDirSkipsWhileMounted(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	host := t.TempDir()
	marker := filepath.Join(host, "keep-me")
	const markerData = "do-not-delete"
	if err := os.WriteFile(marker, []byte(markerData), 0o644); err != nil {
		t.Fatal(err)
	}

	exportRoot := t.TempDir()
	share := filepath.Join(exportRoot, "share")
	if err := os.Mkdir(share, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("mount", "--bind", host, share).CombinedOutput(); err != nil {
		t.Fatalf("bind mount: %s: %v", out, err)
	}
	defer func() {
		unmountShare(share)
		_ = os.RemoveAll(exportRoot)
	}()

	if !isMountPoint(share) {
		t.Fatal("expected share to be a mount point after bind")
	}

	// Force the still-mounted path: unmount is a no-op.
	removeExportDirWith(exportRoot, func(string) {})

	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("host marker wiped or missing: %v", err)
	}
	if string(got) != markerData {
		t.Fatalf("host marker corrupted: %q", got)
	}
	if !isMountPoint(share) {
		t.Fatal("share should still be mounted after refused RemoveAll")
	}
}

func TestRemoveExportDirAfterUnmountKeepsHost(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	host := t.TempDir()
	marker := filepath.Join(host, "keep-me")
	const markerData = "still-here"
	if err := os.WriteFile(marker, []byte(markerData), 0o644); err != nil {
		t.Fatal(err)
	}

	exportRoot := t.TempDir()
	share := filepath.Join(exportRoot, "share")
	if err := os.Mkdir(share, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("mount", "--bind", host, share).CombinedOutput(); err != nil {
		t.Fatalf("bind mount: %s: %v", out, err)
	}

	removeExportDir(exportRoot)

	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("host marker wiped or missing: %v", err)
	}
	if string(got) != markerData {
		t.Fatalf("host marker corrupted: %q", got)
	}
	if _, err := os.Stat(exportRoot); !os.IsNotExist(err) {
		t.Fatalf("export root should be removed after successful unmount: %v", err)
	}
}
