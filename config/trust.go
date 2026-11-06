package config

import (
	"fmt"
	"os"
	"syscall"
)

// checkDirectoryConfigTrust refuses a Directory config that root should not
// run binaries from: it must be owned by the invoking uid or by root and have
// no group or other write bit.
func checkDirectoryConfigTrust(path string, uid int) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("config file %s: %w", path, err)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		if owner := int(st.Uid); owner != uid && owner != 0 {
			return fmt.Errorf("config file %s: not trusted, wrong owner uid %d (must be owned by the invoking user, uid %d, or by root)", path, owner, uid)
		}
	}
	switch mode := fi.Mode().Perm(); {
	case mode&0o020 != 0:
		return fmt.Errorf("config file %s: not trusted, group-writable (mode %04o)", path, mode)
	case mode&0o002 != 0:
		return fmt.Errorf("config file %s: not trusted, world-writable (mode %04o)", path, mode)
	}
	return nil
}
