package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

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
	// UID is the invoking user's uid. Not used yet.
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
// Config file that exists but can't be read is skipped, as before.
func Resolve(v *viper.Viper, in ResolveInput) (Config, []string, error) {
	setDefaults(v)

	var files []string
	if in.ConfigFile != "" {
		if err := readLayer(v, in.ConfigFile); err != nil {
			return Default(), nil, err
		}
		files = []string{in.ConfigFile}
	} else {
		for _, f := range searchFiles(in) {
			if readLayer(v, f) == nil {
				files = append(files, f)
			}
		}
	}

	c := Default()
	if err := v.Unmarshal(&c); err != nil {
		return c, files, err
	}
	// viper lowercases env names, so re-read every file used and merge env
	// per key in order; later files win.
	c.Env = map[string]string{}
	for _, f := range files {
		env, err := rawConfigEnv(f)
		if err != nil {
			return c, files, err
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

// readLayer merges one Config file into v on top of what is already there.
func readLayer(v *viper.Viper, path string) error {
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	return v.MergeInConfig()
}

// searchFiles lists the Config files to read without --config, lowest
// precedence first: the User config, then the Directory config. Only files
// that exist are returned.
func searchFiles(in ResolveInput) []string {
	var user, dir string
	if in.Home != "" {
		user = filepath.Join(in.Home, ".fcvm.yaml")
	}
	if in.Cwd != "" {
		dir = filepath.Join(in.Cwd, ".fcvm.yaml")
	}
	if user != "" && dir != "" && sameFile(user, dir) {
		dir = ""
	}
	var out []string
	for _, f := range []string{user, dir} {
		if f == "" {
			continue
		}
		if fi, err := os.Stat(f); err == nil && fi.Mode().IsRegular() {
			out = append(out, f)
		}
	}
	return out
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

// rawConfigEnv re-reads the Config file's env section directly, bypassing
// viper's key-lowercasing (viper lowercases every key it reads, including
// map values nested under env:) so uppercase env var names survive.
func rawConfigEnv(path string) (map[string]string, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
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
