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
