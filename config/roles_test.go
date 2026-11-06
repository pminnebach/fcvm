package config

import (
	"path/filepath"
	"testing"
)

func TestFileRoleNamesEachFile(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	user := filepath.Join(home, ".fcvm.yaml")
	dir := filepath.Join(cwd, ".fcvm.yaml")
	in := ResolveInput{Cwd: cwd, Home: home}
	if got := FileRole(in, user); got != "User config" {
		t.Errorf("user role = %q", got)
	}
	if got := FileRole(in, dir); got != "Directory config" {
		t.Errorf("dir role = %q", got)
	}
	in.ConfigFile = "/x/y.yaml"
	if got := FileRole(in, "/x/y.yaml"); got != "explicit --config" {
		t.Errorf("explicit role = %q", got)
	}
}
