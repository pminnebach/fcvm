package rootfs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectHooksSystemdUnit(t *testing.T) {
	dir := t.TempDir()
	if err := InjectHooks(dir); err != nil {
		t.Fatal(err)
	}
	unit := filepath.Join(dir, "etc/systemd/system/fcvm-start.service")
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("missing systemd unit: %v", err)
	}
	link := filepath.Join(dir, "etc/systemd/system/multi-user.target.wants/fcvm-start.service")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("missing enabled unit symlink: %v", err)
	}
	if target != "../fcvm-start.service" {
		t.Fatalf("symlink target = %q", target)
	}
	mmds, err := os.ReadFile(filepath.Join(dir, "usr/local/bin/fcvm-mmds.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mmds), "Accept: application/json") {
		t.Fatal("fcvm-mmds.sh must request JSON from MMDS")
	}
}

func TestInjectGuestAgent(t *testing.T) {
	dir := t.TempDir()
	agent := filepath.Join(t.TempDir(), "fcvm-guest-agent")
	if err := os.WriteFile(agent, []byte("#!/bin/true\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := InjectGuestAgent(dir, agent); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "usr/local/bin/fcvm-guest-agent")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("missing agent binary: %v", err)
	}
	unit := filepath.Join(dir, "etc/systemd/system/fcvm-guest-agent.service")
	data, err := os.ReadFile(unit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "fcvm-guest-agent") {
		t.Fatalf("unit = %s", data)
	}
	link := filepath.Join(dir, "etc/systemd/system/multi-user.target.wants/fcvm-guest-agent.service")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if target != "../fcvm-guest-agent.service" {
		t.Fatalf("symlink = %q", target)
	}
}
