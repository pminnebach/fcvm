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
}
