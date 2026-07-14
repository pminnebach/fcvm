package assets

import (
	"os"
	"path/filepath"

	"github.com/fcvm/fcvm/rootfs"
)

func InjectHooks(rootfsDir string) error {
	return rootfs.InjectHooks(rootfsDir)
}

func PatchExt4(ext4Path, sshPubKey string) error {
	dir, err := os.MkdirTemp("", "fcvm-mount-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	mountPoint := filepath.Join(dir, "mnt")
	if err := os.Mkdir(mountPoint, 0o755); err != nil {
		return err
	}
	return rootfs.PatchMounted(mountPoint, ext4Path, sshPubKey)
}
