package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type JailerConfig struct {
	ChrootBaseDir string `mapstructure:"chroot-base-dir"`
	UID           int    `mapstructure:"uid"`
	GID           int    `mapstructure:"gid"`
}

type NetworkConfig struct {
	CNINetwork string `mapstructure:"cni-network"`
	TapIP      string `mapstructure:"tap-ip"`
	GuestIP    string `mapstructure:"guest-ip"`
}

type MountConfig struct {
	Host   string `mapstructure:"host"`
	Guest  string `mapstructure:"guest"`
	Mode   string `mapstructure:"mode"` // ro, rw
	Method string `mapstructure:"method"` // auto, nfs, block
}

type Config struct {
	StateDir        string            `mapstructure:"state-dir"`
	FirecrackerBin  string            `mapstructure:"firecracker-bin"`
	JailerBin       string            `mapstructure:"jailer-bin"`
	Jailer          JailerConfig      `mapstructure:"jailer"`
	Kernel          string            `mapstructure:"kernel"`
	KernelURL       string            `mapstructure:"kernel-url"`
	Rootfs          string            `mapstructure:"rootfs"`
	VCPUCount       int64             `mapstructure:"vcpu-count"`
	MemSizeMib      int64             `mapstructure:"mem-size-mib"`
	Network         NetworkConfig     `mapstructure:"network"`
	Env             map[string]string `mapstructure:"env"`
	Mounts          []MountConfig     `mapstructure:"mounts"`
	ExposeKVM       bool              `mapstructure:"expose-kvm"`
	SSHKey          string            `mapstructure:"ssh-key"`
	WaitTimeoutSec  int               `mapstructure:"wait-timeout"`
	Verbose         bool              `mapstructure:"verbose"`
	VMID            string            `mapstructure:"-"`
}

func Default() Config {
	home, _ := os.UserHomeDir()
	state := filepath.Join(home, ".fcvm")
	return Config{
		StateDir:       state,
		FirecrackerBin: filepath.Join(state, "bin", "firecracker"),
		JailerBin:      filepath.Join(state, "bin", "jailer"),
		Jailer: JailerConfig{
			ChrootBaseDir: filepath.Join(state, "jailer"),
			UID:           1000,
			GID:           1000,
		},
		Kernel:         filepath.Join(state, "images", "vmlinux"),
		Rootfs:         filepath.Join(state, "images", "rootfs.ext4"),
		VCPUCount:      2,
		MemSizeMib:     512,
		WaitTimeoutSec: 120,
		SSHKey:         filepath.Join(state, "id_ed25519"),
		Network: NetworkConfig{
			TapIP:   "172.16.0.1",
			GuestIP: "172.16.0.2",
		},
		Env: map[string]string{},
	}
}

func (c Config) Validate() error {
	if c.FirecrackerBin == "" {
		return fmt.Errorf("firecracker-bin is required")
	}
	if c.JailerBin == "" {
		return fmt.Errorf("jailer-bin is required")
	}
	if c.Kernel == "" {
		return fmt.Errorf("kernel path is required")
	}
	if c.Rootfs == "" {
		return fmt.Errorf("rootfs path is required")
	}
	if c.VCPUCount < 1 {
		return fmt.Errorf("vcpu-count must be >= 1")
	}
	if c.MemSizeMib < 1 {
		return fmt.Errorf("mem-size-mib must be >= 1")
	}
	return nil
}

func (c Config) VMStateDir(id string) string {
	return filepath.Join(c.StateDir, "vms", id)
}
