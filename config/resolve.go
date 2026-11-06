package config

import (
	"fmt"
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
// Without an explicit path the User config (~/.fcvm.yaml) is searched before
// ./.fcvm.yaml, and only the first file found is read. A file that exists but
// can't be read, parsed or decoded is an error naming it; a missing file is
// not (except an explicit --config path).
func Resolve(v *viper.Viper, in ResolveInput) (Config, []string, error) {
	setDefaults(v)

	if in.ConfigFile != "" {
		v.SetConfigFile(in.ConfigFile)
	} else {
		if in.Home != "" {
			v.AddConfigPath(in.Home)
			v.SetConfigName(".fcvm")
			v.SetConfigType("yaml")
		}
		if in.Cwd != "" {
			v.AddConfigPath(in.Cwd)
		}
	}
	var files []string
	if err := v.ReadInConfig(); err == nil {
		files = append(files, v.ConfigFileUsed())
	} else if _, notFound := err.(viper.ConfigFileNotFoundError); !notFound {
		// Found but unusable (syntax error, unreadable), or an explicit
		// --config path that is missing: a hard error naming the file.
		return Default(), nil, fmt.Errorf("config file %s: %w", usedOr(v, in.ConfigFile), err)
	}

	c := Default()
	if err := v.Unmarshal(&c); err != nil {
		return c, files, fmt.Errorf("config file %s: %w", strings.Join(files, ", "), err)
	}
	if c.Env == nil {
		c.Env = map[string]string{}
	}
	if env, err := rawConfigEnv(v.ConfigFileUsed()); err != nil {
		return c, files, err
	} else if env != nil {
		c.Env = env
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

// usedOr returns the file viper tried to read, or fallback if it has none yet.
func usedOr(v *viper.Viper, fallback string) string {
	if f := v.ConfigFileUsed(); f != "" {
		return f
	}
	return fallback
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
