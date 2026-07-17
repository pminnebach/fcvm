package rootfs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func BuildFromDockerfile(dockerfile, tag, outputExt4, size, sshPubKey string) error {
	if size == "" {
		size = "4G"
	}
	dfDir := filepath.Dir(dockerfile)
	if out, err := exec.Command("docker", "build", "-f", dockerfile, "-t", tag, dfDir).CombinedOutput(); err != nil {
		return fmt.Errorf("docker build: %s: %w", out, err)
	}
	dir, err := os.MkdirTemp("", "fcvm-docker-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	root := filepath.Join(dir, "rootfs")
	if err := os.Mkdir(root, 0o755); err != nil {
		return err
	}
	cidOut, err := exec.Command("docker", "create", tag).Output()
	if err != nil {
		return fmt.Errorf("docker create: %w", err)
	}
	cid := string(cidOut[:len(cidOut)-1])
	defer exec.Command("docker", "rm", cid).Run()
	exportPath := filepath.Join(dir, "export.tar")
	if out, err := exec.Command("docker", "export", cid, "-o", exportPath).CombinedOutput(); err != nil {
		return fmt.Errorf("docker export: %s: %w", out, err)
	}
	if out, err := exec.Command("tar", "-xf", exportPath, "-C", root).CombinedOutput(); err != nil {
		return fmt.Errorf("tar extract: %s: %w", out, err)
	}
	if err := InjectHooks(root); err != nil {
		return err
	}
	if sshPubKey != "" {
		if err := InjectSSHKey(root, sshPubKey); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputExt4), 0o755); err != nil {
		return err
	}
	if err := os.Remove(outputExt4); err != nil && !os.IsNotExist(err) {
		return err
	}
	if out, err := exec.Command("truncate", "-s", size, outputExt4).CombinedOutput(); err != nil {
		return fmt.Errorf("truncate: %s: %w", out, err)
	}
	if out, err := exec.Command("mkfs.ext4", "-d", root, "-F", outputExt4).CombinedOutput(); err != nil {
		return fmt.Errorf("mkfs.ext4: %s: %w", out, err)
	}
	return nil
}
