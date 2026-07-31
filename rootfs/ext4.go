package rootfs

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// ext4Overhead pads the measured tree so filesystem metadata and a little
// growth room fit. mkfs.ext4 fails outright on an undersized image.
const (
	ext4Overhead = 40 // percent
	ext4MinBytes = 64 << 20
)

// SizeForDir returns a truncate-compatible size that fits dir plus overhead.
// Callers previously hardcoded 512M or 1G, which silently capped how much a
// user could mount or unpack.
func SizeForDir(dir string) (string, error) {
	var total int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			total += 4096
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("size %q: %w", dir, err)
	}
	total += total * ext4Overhead / 100
	if total < ext4MinBytes {
		total = ext4MinBytes
	}
	return strconv.FormatInt(total, 10), nil
}

// MakeExt4 creates an ext4 image of the given truncate-style size, populated
// from dir.
func MakeExt4(dir, imagePath, size string) error {
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		return err
	}
	if err := os.Remove(imagePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if out, err := exec.Command("truncate", "-s", size, imagePath).CombinedOutput(); err != nil {
		return fmt.Errorf("truncate %s: %s: %w", size, out, err)
	}
	if out, err := exec.Command("mkfs.ext4", "-d", dir, "-F", imagePath).CombinedOutput(); err != nil {
		return fmt.Errorf("mkfs.ext4: %s: %w", out, err)
	}
	return nil
}
