package rootfs

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Sizing from the source tree replaces the old hardcoded 512M/1G limits, so a
// tree larger than those must produce a larger image, and a tiny tree must
// still get a filesystem mkfs.ext4 will accept.
func TestSizeForDir(t *testing.T) {
	dir := t.TempDir()
	small, err := SizeForDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	smallBytes, err := strconv.ParseInt(small, 10, 64)
	if err != nil {
		t.Fatalf("size %q is not a byte count: %v", small, err)
	}
	if smallBytes < ext4MinBytes {
		t.Fatalf("empty tree sized %d, below the %d floor", smallBytes, int64(ext4MinBytes))
	}

	payload := make([]byte, 4<<20)
	for i := range 40 {
		if err := os.WriteFile(filepath.Join(dir, "f"+strconv.Itoa(i)), payload, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	big, err := SizeForDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	bigBytes, _ := strconv.ParseInt(big, 10, 64)
	if bigBytes <= smallBytes {
		t.Fatalf("160MiB of files sized %d, not larger than the empty tree (%d)", bigBytes, smallBytes)
	}
	// Must exceed the old hardcoded 512M cap plus leave headroom for metadata.
	if bigBytes < 160<<20 {
		t.Fatalf("size %d does not cover the %d bytes of content", bigBytes, 160<<20)
	}
}
