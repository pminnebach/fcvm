package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
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
	_ = exec.Command("umount", target).Run()
	_ = os.RemoveAll(target)
	if err := os.Mkdir(target, 0o755); err != nil {
		return nil, err
	}
	if err := exec.Command("mount", "--bind", abs, target).Run(); err != nil {
		return nil, fmt.Errorf("bind mount for nfs export (need root): %w", err)
	}
	uid, gid, err := pathOwnerIDs(abs)
	if err != nil {
		return nil, err
	}
	line := nfsExportLine(target, uid, gid, readOnly) + "\n"
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

// TeardownNFSExportsForVM removes NFS exports for vmID and vmID-N mount slots.
func TeardownNFSExportsForVM(vmID string) {
	parent := filepath.Join("/tmp", "fcvm-exports")
	entries, err := os.ReadDir(parent)
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if name == vmID || strings.HasPrefix(name, vmID+"-") {
				TeardownNFSExport(name)
			}
		}
	}
	TeardownNFSExport(vmID)
}

func pathOwnerIDs(path string) (uid, gid int, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("stat uid/gid for %q", path)
	}
	return int(st.Uid), int(st.Gid), nil
}

func nfsExportLine(target string, uid, gid int, readOnly bool) string {
	rw := "rw"
	if readOnly {
		rw = "ro"
	}
	if runtime.GOOS == "darwin" {
		return fmt.Sprintf("%s -alldirs -mapall=%d -network 172.16.0.0 -mask 255.255.0.0", target, uid)
	}
	return fmt.Sprintf("%s *(%s,sync,no_subtree_check,all_squash,anonuid=%d,anongid=%d)", target, rw, uid, gid)
}
