package assets

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pminnebach/fcvm/rootfs"
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

func PatchNetwork(ext4Path, guestIP, tapIP string) error {
	dir, err := os.MkdirTemp("", "fcvm-mount-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	mountPoint := filepath.Join(dir, "mnt")
	if err := os.Mkdir(mountPoint, 0o755); err != nil {
		return err
	}
	if out, err := exec.Command("mount", "-o", "loop", ext4Path, mountPoint).CombinedOutput(); err != nil {
		return fmt.Errorf("mount ext4: %s: %w", out, err)
	}
	defer exec.Command("umount", mountPoint).Run()
	return rootfs.InjectNetwork(mountPoint, guestIP, tapIP)
}
