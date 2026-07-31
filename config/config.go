package config

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

type JailerConfig struct {
	ChrootBaseDir string   `mapstructure:"chroot-base-dir"`
	UID           int      `mapstructure:"uid"`
	GID           int      `mapstructure:"gid"`
	PerVMUIDs     bool     `mapstructure:"per-vm-uids"`
	NumaNode      int      `mapstructure:"numa-node"`
	Daemonize     bool     `mapstructure:"daemonize"`
	ParentCgroup  string   `mapstructure:"parent-cgroup"`
	Cgroup        []string `mapstructure:"cgroup"`
}

type NetworkConfig struct {
	CNINetwork  string   `mapstructure:"cni-network"`
	TapIP       string   `mapstructure:"tap-ip"`
	GuestIP     string   `mapstructure:"guest-ip"`
	Nameservers []string `mapstructure:"nameservers"`
}

// Mount methods. "auto" resolves to NFS; it never silently degrades to a
// block-device copy, because that discards guest writes.
const (
	MountAuto  = "auto"
	MountNFS   = "nfs"
	MountBlock = "block"
)

type MountConfig struct {
	Host   string `mapstructure:"host"`
	Guest  string `mapstructure:"guest"`
	Mode   string `mapstructure:"mode"`   // ro, rw
	Method string `mapstructure:"method"` // auto, nfs, block
	Size   string `mapstructure:"size"`   // block only; empty sizes from the source tree
}

// ReadOnly reports whether the mount was requested read-only.
func (m MountConfig) ReadOnly() bool { return m.Mode == "ro" }

// ResolvedMethod returns the concrete method for this mount.
func (m MountConfig) ResolvedMethod() string {
	if m.Method == "" || m.Method == MountAuto {
		return MountNFS
	}
	return m.Method
}

type Config struct {
	StateDir       string            `mapstructure:"state-dir"`
	FirecrackerBin string            `mapstructure:"firecracker-bin"`
	JailerBin      string            `mapstructure:"jailer-bin"`
	Jailer         JailerConfig      `mapstructure:"jailer"`
	Kernel         string            `mapstructure:"kernel"`
	KernelURL      string            `mapstructure:"kernel-url"`
	KernelArgs     string            `mapstructure:"kernel-args"`
	Rootfs         string            `mapstructure:"rootfs"`
	LogLevel       string            `mapstructure:"log-level"`
	CPUTemplate    string            `mapstructure:"cpu-template"`
	DisableSMT     bool              `mapstructure:"disable-smt"`
	VCPUCount      int64             `mapstructure:"vcpu-count"`
	MemSizeMib     int64             `mapstructure:"mem-size-mib"`
	Network        NetworkConfig     `mapstructure:"network"`
	Env            map[string]string `mapstructure:"env"`
	Mounts         []MountConfig     `mapstructure:"mounts"`
	SSHKey         string            `mapstructure:"ssh-key"`
	GuestAgentBin  string            `mapstructure:"guest-agent-bin"`
	WaitTimeoutSec int               `mapstructure:"wait-timeout"`
	StopTimeoutSec int               `mapstructure:"stop-timeout"`
	Verbose        bool              `mapstructure:"verbose"`
}

func UserHomeDir() (string, error) {
	if u := os.Getenv("SUDO_USER"); u != "" {
		if entry, err := user.Lookup(u); err == nil && entry.HomeDir != "" {
			return entry.HomeDir, nil
		}
	}
	return os.UserHomeDir()
}

func ExpandPath(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := UserHomeDir()
	if err != nil {
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

func Default() Config {
	home, _ := UserHomeDir()
	state := filepath.Join(home, ".fcvm")
	return Config{
		StateDir:       state,
		FirecrackerBin: filepath.Join(state, "bin", "firecracker"),
		JailerBin:      filepath.Join(state, "bin", "jailer"),
		Jailer: JailerConfig{
			ChrootBaseDir: filepath.Join(state, "jailer"),
			UID:           1000,
			GID:           1000,
			NumaNode:      0,
			Daemonize:     false,
		},
		Kernel:         filepath.Join(state, "images", "vmlinux"),
		KernelArgs:     "console=ttyS0 reboot=k panic=1 net.ifnames=0 biosdevname=0",
		Rootfs:         filepath.Join(state, "images", "rootfs.ext4"),
		LogLevel:       "Info",
		VCPUCount:      2,
		MemSizeMib:     512,
		WaitTimeoutSec: 120,
		StopTimeoutSec: 5,
		SSHKey:         filepath.Join(state, "id_ed25519"),
		GuestAgentBin:  filepath.Join(state, "bin", "fcvm-guest-agent"),
		Network: NetworkConfig{
			TapIP:       "172.16.0.1",
			GuestIP:     "172.16.0.2",
			Nameservers: HostNameservers(),
		},
		Env: map[string]string{},
	}
}

// fallbackNameserver is used only when the host has no resolvers configured.
const fallbackNameserver = "8.8.8.8"

// HostNameservers returns the host's resolvers, so guests inherit the DNS the
// operator already uses instead of a hardcoded public server.
func HostNameservers() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return []string{fallbackNameserver}
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			// A loopback resolver on the host is not reachable from the guest.
			if ip := net.ParseIP(fields[1]); ip == nil || ip.IsLoopback() {
				continue
			}
			out = append(out, fields[1])
		}
	}
	if len(out) == 0 {
		return []string{fallbackNameserver}
	}
	return out
}

var knownCPUTemplates = map[string]struct{}{
	"C3": {}, "T2": {}, "T2S": {}, "T2CL": {}, "T2A": {}, "V1N1": {}, "None": {},
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
	if c.CPUTemplate != "" {
		if _, ok := knownCPUTemplates[c.CPUTemplate]; !ok {
			return fmt.Errorf("cpu-template %q is not a known template", c.CPUTemplate)
		}
	}
	for _, m := range c.Mounts {
		if m.Host == "" {
			return fmt.Errorf("mount host path is required")
		}
		switch m.Method {
		case "", MountAuto, MountNFS, MountBlock:
		default:
			return fmt.Errorf("mount %q: unknown method %q (auto, nfs, block)", m.Host, m.Method)
		}
		switch m.Mode {
		case "", "ro", "rw":
		default:
			return fmt.Errorf("mount %q: unknown mode %q (ro, rw)", m.Host, m.Mode)
		}
	}
	return nil
}

func (c Config) VMStateDir(id string) string {
	return filepath.Join(c.StateDir, "vms", id)
}

// ExportRoot is the staging area for NFS bind mounts. It lives under the
// root-owned state directory rather than world-writable /tmp.
func (c Config) ExportRoot() string {
	return filepath.Join(c.StateDir, "exports")
}
