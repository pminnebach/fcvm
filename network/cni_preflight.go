package network

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/containernetworking/cni/libcni"
)

// tcRedirectTapPlugin is the plugin type that fills in the VM's
// StaticConfiguration (tap name, MAC, IP) from the CNI result. Without it,
// resolveCNIAddrs has nothing to read after Start.
const tcRedirectTapPlugin = "tc-redirect-tap"

// ValidateCNIPrereqs checks that networkName can actually be invoked without
// mutating any host state: its conflist loads from the default CNI config
// dir, every plugin binary it references (including IPAM plugins) is present
// and executable under the default CNI bin dir, and tc-redirect-tap is one of
// them. Call this before touching the host, so a broken CNI setup fails with
// a clear message instead of a raw libcni error mid-Start.
func ValidateCNIPrereqs(networkName string) error {
	return validateCNIPrereqs(networkName, defaultCNIConfDir, []string{defaultCNIBinDir})
}

func validateCNIPrereqs(networkName, confDir string, binDirs []string) error {
	if networkName == "" {
		return nil
	}
	networkConf, err := libcni.LoadConfList(confDir, networkName)
	if err != nil {
		return fmt.Errorf("CNI network %q: %w", networkName, err)
	}

	haveTapPlugin := false
	seen := map[string]bool{}
	var missing []string
	for _, p := range networkConf.Plugins {
		if p.Network == nil {
			continue
		}
		for _, t := range []string{p.Network.Type, p.Network.IPAM.Type} {
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			if t == tcRedirectTapPlugin {
				haveTapPlugin = true
			}
			if !binaryExists(t, binDirs) {
				missing = append(missing, t)
			}
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("CNI network %q: missing plugin binaries %v under %v (see docs/network.md#cni-mode for install instructions)",
			networkName, missing, binDirs)
	}
	if !haveTapPlugin {
		return fmt.Errorf("CNI network %q: conflist has no %q plugin; fcvm requires it to fill in the VM's tap device, MAC and IP from the CNI result",
			networkName, tcRedirectTapPlugin)
	}
	return nil
}

// binaryExists reports whether name is a regular, executable file in any of dirs.
func binaryExists(name string, dirs []string) bool {
	for _, dir := range dirs {
		info, err := os.Stat(filepath.Join(dir, name))
		if err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return true
		}
	}
	return false
}
