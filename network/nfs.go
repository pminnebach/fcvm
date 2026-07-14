package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type NFSExport struct {
	HostPath   string
	GuestPath  string
	ExportPath string
	ReadOnly   bool
}

func SetupNFSExport(hostPath, vmID string, readOnly bool) (*NFSExport, error) {
	abs, err := filepath.Abs(hostPath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("host path %q: %w", abs, err)
	}
	exportDir := filepath.Join("/tmp", "fcvm-exports", vmID)
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return nil, err
	}
	// bind-mount host path into export dir
	target := filepath.Join(exportDir, "share")
	_ = os.Remove(target)
	if err := os.Mkdir(target, 0o755); err != nil {
		return nil, err
	}
	if err := exec.Command("mount", "--bind", abs, target).Run(); err != nil {
		return nil, fmt.Errorf("bind mount for nfs export (need root): %w", err)
	}
	line := fmt.Sprintf("%s *(rw,sync,no_subtree_check,no_root_squash)\n", target)
exportsFile := "/etc/exports.d/fcvm-" + vmID + ".exports"
	if err := os.WriteFile(exportsFile, []byte(line), 0o644); err != nil {
		return nil, fmt.Errorf("write exports (need root): %w", err)
	}
	if out, err := exec.Command("exportfs", "-ra").CombinedOutput(); err != nil {
		return nil, fmt.Errorf("exportfs: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return &NFSExport{
		HostPath:   abs,
		GuestPath:  "",
		ExportPath: target,
		ReadOnly:   readOnly,
	}, nil
}

func TeardownNFSExport(vmID string) {
	exportsFile := "/etc/exports.d/fcvm-" + vmID + ".exports"
	_ = os.Remove(exportsFile)
	_ = exec.Command("exportfs", "-ra").Run()
	exportDir := filepath.Join("/tmp", "fcvm-exports", vmID, "share")
	_ = exec.Command("umount", exportDir).Run()
	_ = os.RemoveAll(filepath.Join("/tmp", "fcvm-exports", vmID))
}
