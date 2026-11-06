package config

import (
	"fmt"
	"os"
	"syscall"
)

// checkDirectoryConfigTrust refuses a Directory config that root should not
// run binaries from: it must be owned by the invoking uid or by root and have
// no group or other write bit.
// fi must come from the already-open file so the check applies to what is read.
// It fails closed when the owner can't be determined.
func checkDirectoryConfigTrust(path string, fi os.FileInfo, uid int) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("config file %s: not trusted, cannot determine its owner on this platform", path)
	}
	if owner := int(st.Uid); owner != uid && owner != 0 {
		return fmt.Errorf("config file %s: not trusted, wrong owner uid %d (must be owned by the invoking user, uid %d, or by root)", path, owner, uid)
	}
	switch mode := fi.Mode().Perm(); {
	case mode&0o020 != 0:
		return fmt.Errorf("config file %s: not trusted, group-writable (mode %04o)", path, mode)
	case mode&0o002 != 0:
		return fmt.Errorf("config file %s: not trusted, world-writable (mode %04o)", path, mode)
	}
	return nil
}
