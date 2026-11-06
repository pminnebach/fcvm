package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupConfigFiles(t *testing.T, both bool) (user, dir string) {
	t.Helper()
	home, work := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")
	t.Chdir(work)
	user = filepath.Join(home, ".fcvm.yaml")
	dir = filepath.Join(work, ".fcvm.yaml")
	if both {
		for _, p := range []string{user, dir} {
			if err := os.WriteFile(p, []byte("vcpu-count: 2\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return user, dir
}

func skipIfKVM(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/dev/kvm"); err == nil {
		t.Skip("/dev/kvm present; self-check would run for real")
	}
}

func TestSelfCheckPrintsConfigFilesBeforeSkipping(t *testing.T) {
	skipIfKVM(t)
	user, dir := setupConfigFiles(t, true)
	out, err := runCLI(t, t.TempDir(), "self-check")
	if err != nil {
		t.Fatal(err)
	}
	want := "config files: " + user + ", " + dir
	i := strings.Index(out, want)
	if i < 0 {
		t.Fatalf("output %q lacks %q", out, want)
	}
	if i > strings.Index(out, "skip:") {
		t.Fatalf("config line must come before skip: %q", out)
	}
}

func TestSelfCheckPrintsNoneWithoutConfigFiles(t *testing.T) {
	skipIfKVM(t)
	setupConfigFiles(t, false)
	out, err := runCLI(t, t.TempDir(), "self-check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "config files: none") {
		t.Fatalf("output %q lacks none line", out)
	}
}

func TestVerboseLogsEachConfigFileWithRoleInOrder(t *testing.T) {
	user, dir := setupConfigFiles(t, true)
	var buf bytes.Buffer
	debugLog.SetOutput(&buf)
	t.Cleanup(func() {
		debugLog.SetOutput(os.Stderr)
		_ = rootCmd.PersistentFlags().Set("verbose", "false")
	})
	if _, err := runCLI(t, t.TempDir(), "list", "--verbose"); err != nil {
		t.Fatal(err)
	}
	log := buf.String()
	u := strings.Index(log, "User config")
	d := strings.Index(log, "Directory config")
	if u < 0 || d < 0 || u > d || !strings.Contains(log, user) || !strings.Contains(log, dir) {
		t.Fatalf("debug log %q", log)
	}
}
