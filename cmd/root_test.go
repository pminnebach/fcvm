package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	"github.com/pminnebach/fcvm/config"
)

func TestMountFlag(t *testing.T) {
	m, err := mountFlag("/data:/mnt/data:ro")
	if err != nil {
		t.Fatal(err)
	}
	if m.Host != "/data" || m.Guest != "/mnt/data" || m.Mode != "ro" {
		t.Fatalf("unexpected mount: %+v", m)
	}
	if m.Method != config.MountAuto {
		t.Fatalf("method = %q, want auto", m.Method)
	}
}

func TestMountFlagOptions(t *testing.T) {
	m, err := mountFlag("/data:/mnt:method=block,size=2G,ro")
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != config.MountBlock || m.Size != "2G" || m.Mode != "ro" {
		t.Fatalf("unexpected mount: %+v", m)
	}

	m, err = mountFlag("/data:/mnt")
	if err != nil {
		t.Fatal(err)
	}
	if m.Mode != "rw" {
		t.Fatalf("default mode = %q, want rw", m.Mode)
	}
}

// Viper lowercases every key it reads from the config file, including map
// values nested under env:. loadConfig must recover the original casing so
// uppercase env var names from .fcvm.yaml survive, matching the --env CLI
// flag path (which bypasses viper and is unaffected).
func TestLoadConfigPreservesEnvCase(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir := t.TempDir()
	path := filepath.Join(dir, ".fcvm.yaml")
	content := "env:\n  MY_VAR: hello\n  Mixed_Case: world\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatal(err)
	}

	c, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if c.Env["MY_VAR"] != "hello" {
		t.Fatalf("Env[MY_VAR] = %q, want %q (Env: %+v)", c.Env["MY_VAR"], "hello", c.Env)
	}
	if c.Env["Mixed_Case"] != "world" {
		t.Fatalf("Env[Mixed_Case] = %q, want %q (Env: %+v)", c.Env["Mixed_Case"], "world", c.Env)
	}
	if _, ok := c.Env["my_var"]; ok {
		t.Fatalf("Env contains lowercased key my_var, want only original casing (Env: %+v)", c.Env)
	}
}

// A typo in the mode used to fall through to read-write, silently handing the
// guest write access the user did not ask for.
func TestMountFlagRejectsUnknownOptions(t *testing.T) {
	for _, bad := range []string{
		"/data:/mnt:readonly",
		"/data:/mnt:method=virtiofs",
		"/data:/mnt:rx",
		"/data",
		":/mnt:ro",
		"/data::ro",
		"/data:/mnt:ro:extra",
	} {
		if m, err := mountFlag(bad); err == nil {
			t.Errorf("mountFlag(%q) = %+v, want an error", bad, m)
		}
	}
}
