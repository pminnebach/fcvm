package network

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// exportsDir is the NFS exports drop-in directory. A variable so tests can
// redirect it away from the host's real configuration.
var exportsDir = "/etc/exports.d"

type NFSExport struct {
	HostPath   string
	GuestPath  string
	ExportPath string
	ReadOnly   bool
}

// SetupNFSExport bind-mounts hostPath into the VM's export root and exports it
// to a single client. client must be the guest address: an export offered to
// every host on the network is never what the caller wants.
func SetupNFSExport(exportRoot, hostPath, vmID, client string, readOnly bool) (*NFSExport, error) {
	if client == "" {
		return nil, fmt.Errorf("nfs export for %q: no guest address to export to", hostPath)
	}
	abs, err := filepath.Abs(hostPath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("host path %q: %w", abs, err)
	}
	exportDir := ExportDir(exportRoot, vmID)
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return nil, err
	}
	// bind-mount host path into export dir
	target := filepath.Join(exportDir, "share")
	unmountShare(target)
	if IsMountPoint(target) {
		return nil, fmt.Errorf("share still mounted at %s; refuse RemoveAll (would wipe host)", target)
	}
	_ = os.RemoveAll(target)
	if err := os.Mkdir(target, 0o755); err != nil {
		return nil, err
	}
	if err := run("mount", "--bind", abs, target); err != nil {
		return nil, fmt.Errorf("bind mount for nfs export (need root): %w", err)
	}
	uid, gid, err := pathOwnerIDs(abs)
	if err != nil {
		return nil, err
	}
	line := nfsExportLine(target, client, uid, gid, readOnly) + "\n"
	if err := os.MkdirAll(exportsDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(exportsFile(vmID), []byte(line), 0o644); err != nil {
		return nil, fmt.Errorf("write exports (need root): %w", err)
	}
	if err := run("exportfs", "-ra"); err != nil {
		return nil, fmt.Errorf("exportfs: %w", err)
	}
	return &NFSExport{
		HostPath:   abs,
		GuestPath:  "",
		ExportPath: target,
		ReadOnly:   readOnly,
	}, nil
}

// ExportDir is the staging directory for one VM's exports. It lives under the
// state directory, which is root-owned, rather than under world-writable /tmp.
func ExportDir(exportRoot, vmID string) string {
	return filepath.Join(exportRoot, vmID)
}

func exportsFile(vmID string) string {
	return filepath.Join(exportsDir, "fcvm-"+vmID+".exports")
}

func TeardownNFSExport(exportRoot, vmID string) {
	_ = os.Remove(exportsFile(vmID))
	_ = run("exportfs", "-ra")
	removeExportDir(ExportDir(exportRoot, vmID))
}

// TeardownNFSExportsForVM removes NFS exports for vmID and vmID-N mount slots.
func TeardownNFSExportsForVM(exportRoot, vmID string) {
	matched := false
	entries, err := os.ReadDir(exportRoot)
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if name == vmID || strings.HasPrefix(name, vmID+"-") {
				TeardownNFSExport(exportRoot, name)
				matched = matched || name == vmID
			}
		}
	}
	if !matched {
		TeardownNFSExport(exportRoot, vmID)
	}
}

func unmountShare(path string) {
	_ = unix.Unmount(path, 0)
	_ = unix.Unmount(path, unix.MNT_DETACH)
}

// IsMountPoint reports whether path is a mount target in /proc/self/mountinfo.
// Callers about to os.RemoveAll a directory that may hold a bind mount must
// check this first, or the removal recurses into the mount source.
func IsMountPoint(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		// mountinfo: ... - mountpoint ...
		if len(fields) < 5 {
			continue
		}
		if fields[4] == abs {
			return true
		}
	}
	return false
}

// removeExportDir unmounts exportRoot/share and removes exportRoot only if share is not still mounted.
// Leaving a still-mounted share in place avoids os.RemoveAll wiping the host bind source.
func removeExportDir(exportRoot string) {
	removeExportDirWith(exportRoot, unmountShare)
}

func removeExportDirWith(exportRoot string, unmount func(string)) {
	share := filepath.Join(exportRoot, "share")
	unmount(share)
	if IsMountPoint(share) {
		return
	}
	_ = os.RemoveAll(exportRoot)
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

func nfsExportLine(target, client string, uid, gid int, readOnly bool) string {
	rw := "rw"
	if readOnly {
		rw = "ro"
	}
	if runtime.GOOS == "darwin" {
		return fmt.Sprintf("%s -alldirs -mapall=%d -network %s -mask 255.255.255.255", target, uid, client)
	}
	return fmt.Sprintf("%s %s(%s,sync,no_subtree_check,all_squash,anonuid=%d,anongid=%d)", target, client, rw, uid, gid)
}
