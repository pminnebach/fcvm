package assets

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pminnebach/fcvm/network"
	"github.com/pminnebach/fcvm/rootfs"
)

// PatchExt4 applies the guest hooks, SSH key, env and network config to a
// rootfs image in a single loop mount.
func PatchExt4(ext4Path string, opts rootfs.PatchOptions) error {
	dir, err := os.MkdirTemp("", "fcvm-mount-*")
	if err != nil {
		return err
	}
	mountPoint := filepath.Join(dir, "mnt")
	defer cleanupMountTemp(dir, mountPoint)
	if err := os.Mkdir(mountPoint, 0o755); err != nil {
		return err
	}
	return rootfs.PatchMounted(mountPoint, ext4Path, opts)
}

// cleanupMountTemp removes the temp directory, but never while the image is
// still mounted under it: RemoveAll would then delete the image contents.
func cleanupMountTemp(dir, mountPoint string) {
	if network.IsMountPoint(mountPoint) {
		fmt.Fprintf(os.Stderr, "fcvm: %s is still mounted; leaving %s in place\n", mountPoint, dir)
		return
	}
	_ = os.RemoveAll(dir)
}
