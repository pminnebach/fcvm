package network

import (
	"os"
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
