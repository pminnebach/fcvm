package network

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConflist(t *testing.T, dir, filename, name string, pluginTypes []string, ipamType string) {
	t.Helper()
	var plugins []string
	for _, pt := range pluginTypes {
		ipam := ""
		if pt == pluginTypes[0] && ipamType != "" {
			ipam = `,"ipam":{"type":"` + ipamType + `"}`
		}
		plugins = append(plugins, `{"type":"`+pt+`"`+ipam+`}`)
	}
	content := `{"name":"` + name + `","cniVersion":"0.3.1","plugins":[` + strings.Join(plugins, ",") + `]}`
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeExecutable(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCNIPrereqsEmptyNetworkIsNoop(t *testing.T) {
	if err := validateCNIPrereqs("", t.TempDir(), []string{t.TempDir()}); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestValidateCNIPrereqsMissingConflist(t *testing.T) {
	confDir, binDir := t.TempDir(), t.TempDir()
	err := validateCNIPrereqs("fcnet", confDir, []string{binDir})
	if err == nil || !strings.Contains(err.Error(), "fcnet") {
		t.Fatalf("err = %v, want it to name the missing network", err)
	}
}

func TestValidateCNIPrereqsMissingBinary(t *testing.T) {
	confDir, binDir := t.TempDir(), t.TempDir()
	writeConflist(t, confDir, "fcnet.conflist", "fcnet", []string{"ptp", "firewall", "tc-redirect-tap"}, "host-local")
	writeExecutable(t, binDir, "ptp")
	writeExecutable(t, binDir, "tc-redirect-tap")
	// "firewall" and the "host-local" IPAM binary are intentionally missing.

	err := validateCNIPrereqs("fcnet", confDir, []string{binDir})
	if err == nil {
		t.Fatal("expected error for missing plugin binaries")
	}
	if !strings.Contains(err.Error(), "firewall") || !strings.Contains(err.Error(), "host-local") {
		t.Fatalf("err = %v, want it to list firewall and host-local", err)
	}
}

func TestValidateCNIPrereqsMissingTapPlugin(t *testing.T) {
	confDir, binDir := t.TempDir(), t.TempDir()
	writeConflist(t, confDir, "fcnet.conflist", "fcnet", []string{"ptp", "firewall"}, "host-local")
	writeExecutable(t, binDir, "ptp")
	writeExecutable(t, binDir, "firewall")
	writeExecutable(t, binDir, "host-local")

	err := validateCNIPrereqs("fcnet", confDir, []string{binDir})
	if err == nil || !strings.Contains(err.Error(), "tc-redirect-tap") {
		t.Fatalf("err = %v, want mention of missing tc-redirect-tap", err)
	}
}

func TestValidateCNIPrereqsHappyPath(t *testing.T) {
	confDir, binDir := t.TempDir(), t.TempDir()
	writeConflist(t, confDir, "fcnet.conflist", "fcnet", []string{"ptp", "firewall", "tc-redirect-tap"}, "host-local")
	for _, bin := range []string{"ptp", "firewall", "tc-redirect-tap", "host-local"} {
		writeExecutable(t, binDir, bin)
	}

	if err := validateCNIPrereqs("fcnet", confDir, []string{binDir}); err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
}

func TestValidateCNIPrereqsNonExecutableBinary(t *testing.T) {
	confDir, binDir := t.TempDir(), t.TempDir()
	writeConflist(t, confDir, "fcnet.conflist", "fcnet", []string{"ptp", "tc-redirect-tap"}, "")
	writeExecutable(t, binDir, "tc-redirect-tap")
	if err := os.WriteFile(filepath.Join(binDir, "ptp"), []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := validateCNIPrereqs("fcnet", confDir, []string{binDir})
	if err == nil || !strings.Contains(err.Error(), "ptp") {
		t.Fatalf("err = %v, want mention of non-executable ptp", err)
	}
}
