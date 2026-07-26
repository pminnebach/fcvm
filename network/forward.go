package network

import (
	"os"
	"path/filepath"
	"strings"
)

const ipForwardProc = "/proc/sys/net/ipv4/ip_forward"

// ipForwardBackup is where the host's original ip_forward value is parked so
// the last VM to stop can put it back.
func ipForwardBackup(stateDir string) string {
	return filepath.Join(stateDir, ".ip_forward.orig")
}

// EnableIPForward turns on IPv4 forwarding, recording the previous value the
// first time so it can be restored later.
func EnableIPForward(stateDir string) error {
	prev, err := os.ReadFile(ipForwardProc)
	if err != nil {
		return err
	}
	backup := ipForwardBackup(stateDir)
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(backup, prev, 0o644); err != nil {
			return err
		}
	}
	if strings.TrimSpace(string(prev)) == "1" {
		return nil
	}
	return os.WriteFile(ipForwardProc, []byte("1\n"), 0o644)
}

// RestoreIPForward puts ip_forward back to the value recorded before fcvm
// first enabled it. Best effort: a host that never had a backup is left alone.
func RestoreIPForward(stateDir string) {
	backup := ipForwardBackup(stateDir)
	prev, err := os.ReadFile(backup)
	if err != nil {
		return
	}
	if v := strings.TrimSpace(string(prev)); v != "" {
		_ = os.WriteFile(ipForwardProc, []byte(v+"\n"), 0o644)
	}
	_ = os.Remove(backup)
}
