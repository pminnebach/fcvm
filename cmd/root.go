package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pminnebach/fcvm/config"
)

var (
	cfgFile string
	cfg     config.Config
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
	cobra.OnInitialize(initConfig)
	defaults := config.Default()

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: $HOME/.fcvm.yaml)")
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
	rootCmd.PersistentFlags().String("guest-agent-bin", defaults.GuestAgentBin, "guest vsock agent binary to inject into rootfs")
	rootCmd.PersistentFlags().String("cni-network", defaults.Network.CNINetwork, "CNI network name (empty = static TAP)")
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
	_ = viper.BindPFlag("network.cni-network", rootCmd.PersistentFlags().Lookup("cni-network"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	_ = viper.BindPFlag("wait-timeout", rootCmd.PersistentFlags().Lookup("wait-timeout"))
	_ = viper.BindPFlag("stop-timeout", rootCmd.PersistentFlags().Lookup("stop-timeout"))
	_ = viper.BindPFlag("network.nameservers", rootCmd.PersistentFlags().Lookup("nameservers"))

	viper.SetEnvPrefix("FCVM")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()
}

func initConfig() {
	defaults := config.Default()
	viper.SetDefault("state-dir", defaults.StateDir)
	viper.SetDefault("firecracker-bin", defaults.FirecrackerBin)
	viper.SetDefault("jailer-bin", defaults.JailerBin)
	viper.SetDefault("jailer.chroot-base-dir", defaults.Jailer.ChrootBaseDir)
	viper.SetDefault("jailer.uid", defaults.Jailer.UID)
	viper.SetDefault("jailer.gid", defaults.Jailer.GID)
	viper.SetDefault("jailer.per-vm-uids", defaults.Jailer.PerVMUIDs)
	viper.SetDefault("jailer.numa-node", defaults.Jailer.NumaNode)
	viper.SetDefault("jailer.daemonize", defaults.Jailer.Daemonize)
	viper.SetDefault("jailer.parent-cgroup", defaults.Jailer.ParentCgroup)
	viper.SetDefault("kernel", defaults.Kernel)
	viper.SetDefault("kernel-url", defaults.KernelURL)
	viper.SetDefault("kernel-args", defaults.KernelArgs)
	viper.SetDefault("rootfs", defaults.Rootfs)
	viper.SetDefault("log-level", defaults.LogLevel)
	viper.SetDefault("cpu-template", defaults.CPUTemplate)
	viper.SetDefault("disable-smt", defaults.DisableSMT)
	viper.SetDefault("vcpu-count", defaults.VCPUCount)
	viper.SetDefault("mem-size-mib", defaults.MemSizeMib)
	viper.SetDefault("ssh-key", defaults.SSHKey)
	viper.SetDefault("guest-agent-bin", defaults.GuestAgentBin)
	viper.SetDefault("wait-timeout", defaults.WaitTimeoutSec)
	viper.SetDefault("stop-timeout", defaults.StopTimeoutSec)
	viper.SetDefault("network.tap-ip", defaults.Network.TapIP)
	viper.SetDefault("network.guest-ip", defaults.Network.GuestIP)
	viper.SetDefault("network.cni-network", defaults.Network.CNINetwork)
	viper.SetDefault("network.nameservers", defaults.Network.Nameservers)

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := config.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home)
			viper.SetConfigName(".fcvm")
			viper.SetConfigType("yaml")
		}
		viper.AddConfigPath(".")
	}
	_ = viper.ReadInConfig()
}

func loadConfig() (config.Config, error) {
	c := config.Default()
	if err := viper.Unmarshal(&c); err != nil {
		return c, err
	}
	if c.Env == nil {
		c.Env = map[string]string{}
	}
	c.StateDir = config.ExpandPath(c.StateDir)
	c.FirecrackerBin = config.ExpandPath(c.FirecrackerBin)
	c.JailerBin = config.ExpandPath(c.JailerBin)
	c.Jailer.ChrootBaseDir = config.ExpandPath(c.Jailer.ChrootBaseDir)
	c.Kernel = config.ExpandPath(c.Kernel)
	c.Rootfs = config.ExpandPath(c.Rootfs)
	c.SSHKey = config.ExpandPath(c.SSHKey)
	c.GuestAgentBin = config.ExpandPath(c.GuestAgentBin)
	return c, nil
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
