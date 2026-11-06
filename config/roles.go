package config

import "path/filepath"

// FileRole names the role of a Config file that Resolve reported as used for
// in: "explicit --config", "User config" or "Directory config".
func FileRole(in ResolveInput, path string) string {
	if in.ConfigFile != "" {
		return "explicit --config"
	}
	if in.Home != "" && sameFile(path, filepath.Join(in.Home, ConfigFileName)) {
		return "User config"
	}
	return "Directory config"
}
