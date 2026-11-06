package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pminnebach/fcvm/config"
)

var (
	cfgFile string
	cfg     config.Config
	// cfgFiles are the Config files used by this run, in the order read.
	cfgFiles []string
	// debugLog receives debug-level messages, shown with --verbose.
	debugLog = logrus.New()
)

var rootCmd = &cobra.Command{
	Use:   "fcvm",
	Short: "Manage Firecracker microVM lifecycle",
	// Runtime failures (not root, VM missing) should print one line, not the
	// whole usage block. Cobra still prints usage for flag and argument errors.
	SilenceUsage: true,
}

func Execute() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func init() {
	defaults := config.Default()

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "read only this config file, ignoring ./.fcvm.yaml and $HOME/.fcvm.yaml (default: ./.fcvm.yaml layered over $HOME/.fcvm.yaml)")
	rootCmd.PersistentFlags().String("state-dir", defaults.StateDir, "state directory")
	rootCmd.PersistentFlags().String("firecracker-bin", defaults.FirecrackerBin, "firecracker binary path")
	rootCmd.PersistentFlags().String("jailer-bin", defaults.JailerBin, "jailer binary path")
	rootCmd.PersistentFlags().String("kernel", defaults.Kernel, "kernel image path")
	rootCmd.PersistentFlags().String("kernel-args", defaults.KernelArgs, "kernel command line")
	rootCmd.PersistentFlags().String("rootfs", defaults.Rootfs, "rootfs ext4 path")
	rootCmd.PersistentFlags().String("log-level", defaults.LogLevel, "Firecracker log level")
	rootCmd.PersistentFlags().String("cpu-template", defaults.CPUTemplate, "Firecracker CPU template (e.g. C3, T2)")
	rootCmd.PersistentFlags().Bool("disable-smt", false, "disable simultaneous multithreading in the guest")
	rootCmd.PersistentFlags().Int64("vcpu-count", defaults.VCPUCount, "vCPUs per VM")
	rootCmd.PersistentFlags().Int64("mem-size-mib", defaults.MemSizeMib, "memory MiB per VM")
	rootCmd.PersistentFlags().String("ssh-key", defaults.SSHKey, "SSH private key path")
	rootCmd.PersistentFlags().String("guest-agent-bin", defaults.GuestAgentBin, "guest vsock agent binary to inject into rootfs (when --enable-vsock) (experimental)")
	rootCmd.PersistentFlags().Bool("enable-vsock", false, "attach virtio-vsock and inject the guest agent (experimental)")
	rootCmd.PersistentFlags().String("cni-network", defaults.Network.CNINetwork, "CNI network name (empty = static TAP) (experimental)")
	rootCmd.PersistentFlags().Bool("enable-experimental", false, "skip confirmation for experimental commands and flags")
	rootCmd.PersistentFlags().Bool("verbose", false, "verbose logging")
	rootCmd.PersistentFlags().Int("wait-timeout", defaults.WaitTimeoutSec, "seconds to wait for guest SSH")
	rootCmd.PersistentFlags().Int("stop-timeout", defaults.StopTimeoutSec, "seconds to wait for a VM to exit before SIGKILL")
	rootCmd.PersistentFlags().StringSlice("nameservers", defaults.Network.Nameservers, "guest DNS servers (default: host resolvers)")

	_ = viper.BindPFlag("state-dir", rootCmd.PersistentFlags().Lookup("state-dir"))
	_ = viper.BindPFlag("firecracker-bin", rootCmd.PersistentFlags().Lookup("firecracker-bin"))
	_ = viper.BindPFlag("jailer-bin", rootCmd.PersistentFlags().Lookup("jailer-bin"))
	_ = viper.BindPFlag("kernel", rootCmd.PersistentFlags().Lookup("kernel"))
	_ = viper.BindPFlag("kernel-args", rootCmd.PersistentFlags().Lookup("kernel-args"))
	_ = viper.BindPFlag("rootfs", rootCmd.PersistentFlags().Lookup("rootfs"))
	_ = viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))
	_ = viper.BindPFlag("cpu-template", rootCmd.PersistentFlags().Lookup("cpu-template"))
	_ = viper.BindPFlag("disable-smt", rootCmd.PersistentFlags().Lookup("disable-smt"))
	_ = viper.BindPFlag("vcpu-count", rootCmd.PersistentFlags().Lookup("vcpu-count"))
	_ = viper.BindPFlag("mem-size-mib", rootCmd.PersistentFlags().Lookup("mem-size-mib"))
	_ = viper.BindPFlag("ssh-key", rootCmd.PersistentFlags().Lookup("ssh-key"))
	_ = viper.BindPFlag("guest-agent-bin", rootCmd.PersistentFlags().Lookup("guest-agent-bin"))
	_ = viper.BindPFlag("enable-vsock", rootCmd.PersistentFlags().Lookup("enable-vsock"))
	_ = viper.BindPFlag("network.cni-network", rootCmd.PersistentFlags().Lookup("cni-network"))
	_ = viper.BindPFlag("enable-experimental", rootCmd.PersistentFlags().Lookup("enable-experimental"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("wait-timeout", rootCmd.PersistentFlags().Lookup("wait-timeout"))
	_ = viper.BindPFlag("stop-timeout", rootCmd.PersistentFlags().Lookup("stop-timeout"))
	_ = viper.BindPFlag("network.nameservers", rootCmd.PersistentFlags().Lookup("nameservers"))

	viper.SetEnvPrefix("FCVM")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if skipsConfig(cmd) {
			return nil
		}
		c, files, err := resolveConfig()
		if err != nil {
			return err
		}
		cfg, cfgFiles = c, files
		return confirmExperimental(cmd, c, experimentalReasons(cmd, c))
	}
}

// skipsConfig reports whether cmd must run without resolving config, so a
// broken Config file can't break help or shell completion. It checks the
// command and its ancestors, because the command that runs for completion is
// a child (completion bash, zsh, ...) and cmd.Name() is only the leaf.
// __completeNoDesc is an alias of cobra's __complete, so its Name() is
// __complete too.
func skipsConfig(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		switch c.Name() {
		case "experimental", "version", "help", "completion", cobra.ShellCompRequestCmd:
			return true
		}
	}
	return false
}

// resolveConfig gathers this run's inputs and resolves the Config against the
// flag and FCVM_* environment layers bound on the global viper.
func resolveConfig() (config.Config, []string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	home, err := config.UserHomeDir()
	if err != nil {
		home = ""
	}
	uid, err := invokingUID()
	if err != nil {
		return config.Default(), nil, err
	}
	in := config.ResolveInput{
		ConfigFile: cfgFile,
		Cwd:        cwd,
		Home:       home,
		UID:        uid,
	}
	c, files, err := config.Resolve(viper.GetViper(), in)
	if v, _ := rootCmd.PersistentFlags().GetBool("verbose"); v || viper.GetBool("verbose") {
		debugLog.SetLevel(logrus.DebugLevel)
	}
	for _, f := range files {
		debugLog.Debugf("read %s: %s", config.FileRole(in, f), f)
	}
	return c, files, err
}

// invokingUID is the user who ran fcvm: the SUDO_UID user under sudo,
// otherwise the effective uid. An invalid SUDO_UID is an error: falling back
// to the effective uid (root under sudo) would trust the wrong owner.
func invokingUID() (int, error) {
	if s := os.Getenv("SUDO_UID"); s != "" {
		uid, err := strconv.Atoi(s)
		if err != nil || uid < 0 {
			return 0, fmt.Errorf("invalid SUDO_UID %q: must be a non-negative integer", s)
		}
		return uid, nil
	}
	return os.Geteuid(), nil
}

// mountFlag parses host:guest[:opt[,opt...]] where opt is ro, rw,
// method=nfs|block|auto or size=<truncate size>. Unknown options are rejected:
// silently treating a typo like ":readonly" as read-write would hand the guest
// write access the user did not ask for.
func mountFlag(s string) (config.MountConfig, error) {
	const format = "mount format: host:guest[:ro|rw|method=nfs|block|auto|size=N]"
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return config.MountConfig{}, fmt.Errorf("%s", format)
	}
	if parts[0] == "" || parts[1] == "" {
		return config.MountConfig{}, fmt.Errorf("%s", format)
	}
	m := config.MountConfig{
		Host:   parts[0],
		Guest:  parts[1],
		Mode:   "rw",
		Method: config.MountAuto,
	}
	if len(parts) == 2 {
		return m, nil
	}
	for _, opt := range strings.Split(parts[2], ",") {
		key, value, hasValue := strings.Cut(opt, "=")
		switch {
		case !hasValue && (key == "ro" || key == "rw"):
			m.Mode = key
		case key == "method":
			switch value {
			case config.MountAuto, config.MountNFS, config.MountBlock:
				m.Method = value
			default:
				return config.MountConfig{}, fmt.Errorf("mount %q: unknown method %q (auto, nfs, block)", s, value)
			}
		case key == "size" && value != "":
			m.Size = value
		default:
			return config.MountConfig{}, fmt.Errorf("mount %q: unknown option %q; %s", s, opt, format)
		}
	}
	return m, nil
}
