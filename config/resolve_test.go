package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func writeConfigFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, ".fcvm.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveReadsUserConfig(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	path := writeConfigFile(t, home, "vcpu-count: 4\nkernel: ~/images/vmlinux\n")

	c, files, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.VCPUCount != 4 {
		t.Fatalf("VCPUCount = %d, want 4", c.VCPUCount)
	}
	if want := filepath.Join(home, "images", "vmlinux"); c.Kernel != want {
		t.Fatalf("Kernel = %q, want %q", c.Kernel, want)
	}
	if !reflect.DeepEqual(files, []string{path}) {
		t.Fatalf("files = %v, want [%s]", files, path)
	}
}

func TestResolveReadsFileOnlyInCwd(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	path := writeConfigFile(t, cwd, "mem-size-mib: 2048\n")

	c, files, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.MemSizeMib != 2048 {
		t.Fatalf("MemSizeMib = %d, want 2048", c.MemSizeMib)
	}
	if !reflect.DeepEqual(files, []string{path}) {
		t.Fatalf("files = %v, want [%s]", files, path)
	}
}

// Today only one Config file is read: when both exist the User config wins and
// the file in the current directory is ignored entirely.
func TestResolveUserConfigWinsWhenBothExist(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	userPath := writeConfigFile(t, home, "vcpu-count: 4\n")
	writeConfigFile(t, cwd, "vcpu-count: 8\nmem-size-mib: 2048\n")

	c, files, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.VCPUCount != 4 || c.MemSizeMib != 512 {
		t.Fatalf("VCPUCount, MemSizeMib = %d, %d, want 4, 512", c.VCPUCount, c.MemSizeMib)
	}
	if !reflect.DeepEqual(files, []string{userPath}) {
		t.Fatalf("files = %v, want [%s]", files, userPath)
	}
}

func TestResolveWithoutFilesGivesDefaults(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()

	c, files, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c, Default()) {
		t.Fatalf("config = %+v, want defaults %+v", c, Default())
	}
	if len(files) != 0 {
		t.Fatalf("files = %v, want none", files)
	}
}

func TestResolveExplicitConfigReadsOnlyThatFile(t *testing.T) {
	home, cwd, other := t.TempDir(), t.TempDir(), t.TempDir()
	writeConfigFile(t, home, "vcpu-count: 4\n")
	writeConfigFile(t, cwd, "mem-size-mib: 2048\n")
	explicit := filepath.Join(other, "custom.yaml")
	if err := os.WriteFile(explicit, []byte("log-level: Debug\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c, files, err := Resolve(viper.New(), ResolveInput{ConfigFile: explicit, Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.LogLevel != "Debug" || c.VCPUCount != 2 || c.MemSizeMib != 512 {
		t.Fatalf("LogLevel, VCPUCount, MemSizeMib = %q, %d, %d, want Debug, 2, 512", c.LogLevel, c.VCPUCount, c.MemSizeMib)
	}
	if !reflect.DeepEqual(files, []string{explicit}) {
		t.Fatalf("files = %v, want [%s]", files, explicit)
	}
}

func TestResolveMissingExplicitConfigIsAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.yaml")
	if _, _, err := Resolve(viper.New(), ResolveInput{ConfigFile: missing, Cwd: t.TempDir(), Home: t.TempDir()}); err == nil {
		t.Fatal("want an error for a missing explicit Config file")
	}
}

func TestResolveKeepsEnvCasing(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	writeConfigFile(t, home, "env:\n  MY_VAR: hello\n  Mixed_Case: world\n")

	c, _, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"MY_VAR": "hello", "Mixed_Case": "world"}
	if !reflect.DeepEqual(c.Env, want) {
		t.Fatalf("Env = %v, want %v", c.Env, want)
	}
}

func TestResolveFlagsAndEnvOverrideFile(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	writeConfigFile(t, home, "vcpu-count: 4\nmem-size-mib: 2048\n")

	flags := pflag.NewFlagSet("fcvm", pflag.ContinueOnError)
	flags.Int64("vcpu-count", 2, "")
	if err := flags.Parse([]string{"--vcpu-count", "6"}); err != nil {
		t.Fatal(err)
	}
	v := viper.New()
	if err := v.BindPFlag("vcpu-count", flags.Lookup("vcpu-count")); err != nil {
		t.Fatal(err)
	}
	v.SetEnvPrefix("FCVM")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()
	t.Setenv("FCVM_MEM_SIZE_MIB", "4096")

	c, _, err := Resolve(v, ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.VCPUCount != 6 {
		t.Fatalf("VCPUCount = %d, want 6 from the flag", c.VCPUCount)
	}
	if c.MemSizeMib != 4096 {
		t.Fatalf("MemSizeMib = %d, want 4096 from FCVM_MEM_SIZE_MIB", c.MemSizeMib)
	}
}
