package config

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
)

func TestUserHomeDir(t *testing.T) {
	t.Setenv("SUDO_USER", "")
	home, err := UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if home != want {
		t.Fatalf("got %q want %q", home, want)
	}
}

func TestUserHomeDirSudo(t *testing.T) {
	entry, err := user.Lookup("root")
	if err != nil {
		t.Skip("no root user")
	}
	t.Setenv("SUDO_USER", "root")
	home, err := UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if home != entry.HomeDir {
		t.Fatalf("got %q want %q", home, entry.HomeDir)
	}
}

func TestExpandPath(t *testing.T) {
	t.Setenv("SUDO_USER", "root")
	entry, err := user.Lookup("root")
	if err != nil {
		t.Skip("no root user")
	}

	tests := []struct {
		in, want string
	}{
		{"/abs/path", "/abs/path"},
		{"", ""},
		{"~", entry.HomeDir},
		{"~/.fcvm/kernel", filepath.Join(entry.HomeDir, ".fcvm/kernel")},
	}
	for _, tc := range tests {
		if got := ExpandPath(tc.in); got != tc.want {
			t.Errorf("ExpandPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateCPUTemplate(t *testing.T) {
	c := Default()
	c.CPUTemplate = "T2"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.CPUTemplate = "nope"
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for unknown cpu-template")
	}
}

func TestDefaultMachineKnobs(t *testing.T) {
	c := Default()
	if c.KernelArgs == "" || c.LogLevel != "Info" {
		t.Fatalf("defaults: kernel-args=%q log-level=%q", c.KernelArgs, c.LogLevel)
	}
	if c.Jailer.NumaNode != 0 || c.Jailer.Daemonize {
		t.Fatalf("jailer defaults: numa=%d daemonize=%v", c.Jailer.NumaNode, c.Jailer.Daemonize)
	}
	if c.StopTimeoutSec <= 0 {
		t.Fatalf("stop-timeout = %d, want a positive default", c.StopTimeoutSec)
	}
	if len(c.Network.Nameservers) == 0 {
		t.Fatal("nameservers default is empty; guests would have no DNS")
	}
}

func TestValidateMounts(t *testing.T) {
	base := Default()
	base.Mounts = []MountConfig{{Host: "/data", Guest: "/mnt", Mode: "ro", Method: MountBlock}}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid mount rejected: %v", err)
	}
	for _, bad := range []MountConfig{
		{Host: "", Guest: "/mnt"},
		{Host: "/data", Method: "virtiofs"},
		{Host: "/data", Mode: "readonly"},
	} {
		c := Default()
		c.Mounts = []MountConfig{bad}
		if err := c.Validate(); err == nil {
			t.Errorf("Validate accepted %+v", bad)
		}
	}
}

// auto must mean NFS. It used to fall back to a block-device copy, which
// silently discarded the guest's writes.
func TestResolvedMethod(t *testing.T) {
	for in, want := range map[string]string{"": MountNFS, MountAuto: MountNFS, MountNFS: MountNFS, MountBlock: MountBlock} {
		if got := (MountConfig{Method: in}).ResolvedMethod(); got != want {
			t.Errorf("ResolvedMethod(%q) = %q, want %q", in, got, want)
		}
	}
}

// A loopback resolver on the host is unreachable from the guest, so it must
// not be handed to it.
func TestHostNameserversSkipsLoopback(t *testing.T) {
	ns := HostNameservers()
	if len(ns) == 0 {
		t.Fatal("expected at least a fallback nameserver")
	}
	for _, s := range ns {
		if s == "127.0.0.1" || s == "::1" {
			t.Fatalf("loopback resolver %q handed to the guest", s)
		}
	}
}
