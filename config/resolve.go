package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// ConfigFileName is the name of the User config and the Directory config.
const ConfigFileName = ".fcvm.yaml"

// ResolveInput is everything config resolution needs from the environment of
// the run, passed in so resolution does not read process-global state.
type ResolveInput struct {
	// ConfigFile is the explicit --config path; empty means search for a
	// Config file.
	ConfigFile string
	// Cwd is the current directory; empty when it can't be determined.
	Cwd string
	// Home is the user's home (UserHomeDir, SUDO_USER-aware); empty when it
	// can't be determined.
	Home string
	// UID is the invoking user's uid (SUDO_UID under sudo, else the effective
	// uid); a Directory config must be owned by it or by root.
	UID int
}

// Resolve builds the run's Config from the built-in defaults, the Config file
// and the flag and FCVM_* environment layers already bound on v. It returns
// the resolved Config (paths expanded, env casing preserved) and the Config
// files used, in order.
//
// With an explicit path only that file is read and a failure to read it is an
// error. Otherwise the Directory config (./.fcvm.yaml in the exact current
// directory, never its parents) is layered over the User config
// (~/.fcvm.yaml): a key the Directory config sets wins, a key it leaves out
// comes from the User config, maps merge per key and lists are replaced. When
// the current directory is the home directory the file is read once, as the
// User config. A missing current directory means no Directory config. A
// Config file that exists but can't be read, parsed or decoded is an error
// naming it (each file is also decoded on its own, so a wrong-typed value
// reports the first file that fails); a missing file is not, except an
// explicit --config path.
func Resolve(v *viper.Viper, in ResolveInput) (Config, []string, error) {
	setDefaults(v)

	var layers []layer
	if in.ConfigFile != "" {
		l, err := loadLayer(in.ConfigFile, nil)
		if err != nil {
			return Default(), nil, err
		}
		layers = []layer{l}
	} else {
		found, err := searchFiles(in)
		if err != nil {
			return Default(), nil, err
		}
		for _, f := range found {
			var trustUID *int
			dirCfg := isDirectoryConfig(in, f)
			if dirCfg {
				trustUID = &in.UID
			}
			l, err := loadLayer(f, trustUID)
			if err != nil {
				return Default(), nil, err
			}
			if dirCfg {
				if err := refuseStateDir(l); err != nil {
					return Default(), nil, err
				}
			}
			layers = append(layers, l)
		}
	}

	var files []string
	for _, l := range layers {
		v.SetConfigType("yaml")
		if err := v.MergeConfig(bytes.NewReader(l.data)); err != nil {
			return Default(), nil, fmt.Errorf("config file %s: %w", l.path, err)
		}
		files = append(files, l.path)
	}

	c := Default()
	if err := v.Unmarshal(&c); err != nil {
		return c, files, fmt.Errorf("config file %s: %w", strings.Join(files, ", "), err)
	}
	// viper lowercases env names, so take env straight from each file's
	// bytes and merge per key in order; later files win.
	c.Env = map[string]string{}
	for _, l := range layers {
		env, err := rawConfigEnv(l.data)
		if err != nil {
			return c, files, fmt.Errorf("config file %s: %w", l.path, err)
		}
		for k, val := range env {
			c.Env[k] = val
		}
	}
	expand := func(p string) string { return expandPathIn(in.Home, p) }
	c.StateDir = expand(c.StateDir)
	c.FirecrackerBin = expand(c.FirecrackerBin)
	c.JailerBin = expand(c.JailerBin)
	c.Jailer.ChrootBaseDir = expand(c.Jailer.ChrootBaseDir)
	c.Kernel = expand(c.Kernel)
	c.Rootfs = expand(c.Rootfs)
	c.SSHKey = expand(c.SSHKey)
	c.GuestAgentBin = expand(c.GuestAgentBin)
	return c, files, nil
}

// layer is one Config file's path and the bytes read from it, once.
type layer struct {
	path string
	data []byte
	own  *viper.Viper
}

// loadLayer opens path once, and when trustUID is set checks the open file's
// owner and mode before reading it, so the bytes used are the bytes checked.
// It then parses and decodes the bytes on their own so any failure names
// this file.
func loadLayer(path string, trustUID *int) (layer, error) {
	f, err := os.Open(path)
	if err != nil {
		return layer{}, fmt.Errorf("config file %s: %w", path, err)
	}
	defer f.Close()
	if trustUID != nil {
		fi, err := f.Stat()
		if err != nil {
			return layer{}, fmt.Errorf("config file %s: %w", path, err)
		}
		if err := checkDirectoryConfigTrust(path, fi, *trustUID); err != nil {
			return layer{}, err
		}
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return layer{}, fmt.Errorf("config file %s: %w", path, err)
	}
	own := viper.New()
	own.SetConfigType("yaml")
	if err := own.ReadConfig(bytes.NewReader(data)); err != nil {
		return layer{}, fmt.Errorf("config file %s: %w", path, err)
	}
	if err := own.Unmarshal(&Config{}); err != nil {
		return layer{}, fmt.Errorf("config file %s: %w", path, err)
	}
	return layer{path: path, data: data, own: own}, nil
}

// searchFiles lists the Config files to read without --config, lowest
// precedence first: the User config, then the Directory config. A missing
// file is skipped; one that exists but is not a regular file (or a symlink to
// one) is an error naming it. Symlinks are followed.
func searchFiles(in ResolveInput) ([]string, error) {
	var user, dir string
	if in.Home != "" {
		user = filepath.Join(in.Home, ConfigFileName)
	}
	if in.Cwd != "" {
		dir = filepath.Join(in.Cwd, ConfigFileName)
	}
	if user != "" && dir != "" && sameFile(user, dir) {
		dir = ""
	}
	var out []string
	for _, f := range []string{user, dir} {
		if f == "" {
			continue
		}
		if _, err := os.Lstat(f); err != nil {
			continue // missing (or unreachable directory): no file
		}
		fi, err := os.Stat(f)
		if err != nil {
			return nil, fmt.Errorf("config file %s: exists but cannot be used: %w", f, err)
		}
		if !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("config file %s: exists but cannot be used: not a regular file", f)
		}
		out = append(out, f)
	}
	return out, nil
}

// isDirectoryConfig reports whether path, a file from searchFiles, is the
// Directory config: the file found by the current-directory search, not the
// --config file and not the User config (when the current directory is the
// home directory the one file is the User config).
func isDirectoryConfig(in ResolveInput, path string) bool {
	if in.ConfigFile != "" || in.Cwd == "" {
		return false
	}
	if in.Home != "" && sameFile(path, filepath.Join(in.Home, ConfigFileName)) {
		return false
	}
	return sameFile(path, filepath.Join(in.Cwd, ConfigFileName))
}

// refuseStateDir fails when the Directory config l sets state-dir. It checks
// that file on its own, so state-dir from a flag, FCVM_* variable or the User
// config never trips it. One state directory per user keeps list/stop/delete
// consistent and network slots from being handed out twice.
func refuseStateDir(l layer) error {
	if l.own.InConfig("state-dir") {
		return fmt.Errorf("config file %s: state-dir is not allowed in a Directory config (set it in the User config, with --config, or with a flag or FCVM_STATE_DIR)", l.path)
	}
	return nil
}

func sameFile(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	return err == nil && os.SameFile(fa, fb)
}

func setDefaults(v *viper.Viper) {
	defaults := Default()
	v.SetDefault("state-dir", defaults.StateDir)
	v.SetDefault("firecracker-bin", defaults.FirecrackerBin)
	v.SetDefault("jailer-bin", defaults.JailerBin)
	v.SetDefault("jailer.chroot-base-dir", defaults.Jailer.ChrootBaseDir)
	v.SetDefault("jailer.uid", defaults.Jailer.UID)
	v.SetDefault("jailer.gid", defaults.Jailer.GID)
	v.SetDefault("jailer.per-vm-uids", defaults.Jailer.PerVMUIDs)
	v.SetDefault("jailer.numa-node", defaults.Jailer.NumaNode)
	v.SetDefault("jailer.daemonize", defaults.Jailer.Daemonize)
	v.SetDefault("jailer.parent-cgroup", defaults.Jailer.ParentCgroup)
	v.SetDefault("kernel", defaults.Kernel)
	v.SetDefault("kernel-url", defaults.KernelURL)
	v.SetDefault("kernel-args", defaults.KernelArgs)
	v.SetDefault("rootfs", defaults.Rootfs)
	v.SetDefault("log-level", defaults.LogLevel)
	v.SetDefault("cpu-template", defaults.CPUTemplate)
	v.SetDefault("disable-smt", defaults.DisableSMT)
	v.SetDefault("vcpu-count", defaults.VCPUCount)
	v.SetDefault("mem-size-mib", defaults.MemSizeMib)
	v.SetDefault("ssh-key", defaults.SSHKey)
	v.SetDefault("guest-agent-bin", defaults.GuestAgentBin)
	v.SetDefault("enable-vsock", defaults.EnableVsock)
	v.SetDefault("enable-experimental", defaults.EnableExperimental)
	v.SetDefault("wait-timeout", defaults.WaitTimeoutSec)
	v.SetDefault("stop-timeout", defaults.StopTimeoutSec)
	v.SetDefault("network.tap-ip", defaults.Network.TapIP)
	v.SetDefault("network.guest-ip", defaults.Network.GuestIP)
	v.SetDefault("network.cni-network", defaults.Network.CNINetwork)
	v.SetDefault("network.nameservers", defaults.Network.Nameservers)
}

// rawConfigEnv reads the Config file's env section directly from its bytes,
// bypassing viper's key-lowercasing (viper lowercases every key it reads,
// including map values nested under env:) so uppercase env var names survive.
func rawConfigEnv(data []byte) (map[string]string, error) {
	var doc struct {
		Env map[string]string `yaml:"env"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc.Env, nil
}

// expandPathIn expands a leading ~ against home. An empty home leaves the
// path as is.
func expandPathIn(home, path string) string {
	if path == "" || path[0] != '~' || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
