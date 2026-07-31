package vm

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

// lockState serializes VM index allocation across concurrent fcvm processes.
// Allocation reads every state file and then writes a new one; without the
// lock two starts can pick the same index and collide on TAP name and address.
func lockState(stateDir string) (func(), error) {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(stateDir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	// Idempotent so callers can release early and still defer it.
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
			_ = f.Close()
		})
	}, nil
}
