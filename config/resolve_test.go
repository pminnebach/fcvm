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

func TestResolveDirectoryConfigWinsAndFallsThrough(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	userPath := writeConfigFile(t, home, "vcpu-count: 4\nmem-size-mib: 1024\n")
	dirPath := writeConfigFile(t, cwd, "vcpu-count: 8\n")

	c, files, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.VCPUCount != 8 {
		t.Fatalf("VCPUCount = %d, want 8 from the Directory config", c.VCPUCount)
	}
	if c.MemSizeMib != 1024 {
		t.Fatalf("MemSizeMib = %d, want 1024 from the User config", c.MemSizeMib)
	}
	if c.StopTimeoutSec != Default().StopTimeoutSec {
		t.Fatalf("StopTimeoutSec = %d, want the default", c.StopTimeoutSec)
	}
	if want := []string{userPath, dirPath}; !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
}

func TestResolveMergesEnvPerKeyKeepingCasing(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	writeConfigFile(t, home, "env:\n  SHARED: user\n  USER_ONLY: u\n")
	writeConfigFile(t, cwd, "env:\n  SHARED: dir\n  Dir_Only: d\n")

	c, _, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"SHARED": "dir", "USER_ONLY": "u", "Dir_Only": "d"}
	if !reflect.DeepEqual(c.Env, want) {
		t.Fatalf("Env = %v, want %v", c.Env, want)
	}
}

func TestResolveReplacesLists(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	writeConfigFile(t, home, "network:\n  nameservers: [1.1.1.1, 1.0.0.1]\n  tap-ip: 172.16.5.1\n")
	writeConfigFile(t, cwd, "network:\n  nameservers: [9.9.9.9]\n")

	c, _, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"9.9.9.9"}; !reflect.DeepEqual(c.Network.Nameservers, want) {
		t.Fatalf("Nameservers = %v, want %v", c.Network.Nameservers, want)
	}
	if c.Network.TapIP != "172.16.5.1" {
		t.Fatalf("TapIP = %q, want it kept from the User config", c.Network.TapIP)
	}
}

func TestResolveCwdEqualToHomeReadsFileOnce(t *testing.T) {
	home := t.TempDir()
	path := writeConfigFile(t, home, "vcpu-count: 4\nenv:\n  A: b\n")

	c, files, err := Resolve(viper.New(), ResolveInput{Cwd: home, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(files, []string{path}) {
		t.Fatalf("files = %v, want [%s] once", files, path)
	}
	if c.VCPUCount != 4 {
		t.Fatalf("VCPUCount = %d, want 4", c.VCPUCount)
	}
}

func TestResolveMissingCwdGivesNoDirectoryConfigAndNoError(t *testing.T) {
	home := t.TempDir()
	userPath := writeConfigFile(t, home, "vcpu-count: 4\n")

	for _, cwd := range []string{"", filepath.Join(t.TempDir(), "gone")} {
		c, files, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
		if err != nil {
			t.Fatalf("cwd %q: %v", cwd, err)
		}
		if c.VCPUCount != 4 || !reflect.DeepEqual(files, []string{userPath}) {
			t.Fatalf("cwd %q: VCPUCount, files = %d, %v", cwd, c.VCPUCount, files)
		}
	}
}

func TestResolveDoesNotCheckParentDirectories(t *testing.T) {
	home, parent := t.TempDir(), t.TempDir()
	writeConfigFile(t, parent, "vcpu-count: 8\n")
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}

	c, files, err := Resolve(viper.New(), ResolveInput{Cwd: child, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.VCPUCount != 2 || len(files) != 0 {
		t.Fatalf("VCPUCount, files = %d, %v, want 2 and none", c.VCPUCount, files)
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

func TestResolveSyntaxErrorNamesFile(t *testing.T) {
	for _, where := range []string{"user", "directory"} {
		t.Run(where, func(t *testing.T) {
			home, cwd := t.TempDir(), t.TempDir()
			dir := home
			if where == "directory" {
				dir = cwd
			}
			path := writeConfigFile(t, dir, "vcpu-count: [unclosed\n")

			_, _, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
			if err == nil {
				t.Fatal("want an error for a YAML syntax error")
			}
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("error %q does not name %s", err, path)
			}
		})
	}
}

func TestResolveWrongTypeNamesFile(t *testing.T) {
	t.Run("user config, directory config valid", func(t *testing.T) {
		home, cwd := t.TempDir(), t.TempDir()
		bad := writeConfigFile(t, home, "vcpu-count: \"four\"\n")
		writeConfigFile(t, cwd, "mem-size-mib: 1024\n")
		_, _, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
		if err == nil || !strings.Contains(err.Error(), bad) {
			t.Fatalf("want an error naming %s, got %v", bad, err)
		}
	})
	t.Run("directory config", func(t *testing.T) {
		home, cwd := t.TempDir(), t.TempDir()
		bad := writeConfigFile(t, cwd, "vcpu-count: \"four\"\n")
		_, _, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
		if err == nil || !strings.Contains(err.Error(), bad) {
			t.Fatalf("want an error naming %s, got %v", bad, err)
		}
	})
	t.Run("explicit config, user config valid", func(t *testing.T) {
		home, other := t.TempDir(), t.TempDir()
		writeConfigFile(t, home, "mem-size-mib: 1024\n")
		bad := writeConfigFile(t, other, "vcpu-count: \"four\"\n")
		_, _, err := Resolve(viper.New(), ResolveInput{ConfigFile: bad, Cwd: t.TempDir(), Home: home, UID: os.Getuid()})
		if err == nil || !strings.Contains(err.Error(), bad) {
			t.Fatalf("want an error naming %s, got %v", bad, err)
		}
	})
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

func TestResolveRefusesStateDirInDirectoryConfig(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	path := writeConfigFile(t, cwd, "state-dir: /elsewhere\n")

	_, _, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err == nil {
		t.Fatal("want an error for state-dir in a Directory config")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "state-dir") {
		t.Fatalf("error %q should name the file %s and state-dir", err, path)
	}
}

func TestResolveAllowsStateDirInUserAndExplicitConfig(t *testing.T) {
	home, cwd, other := t.TempDir(), t.TempDir(), t.TempDir()
	writeConfigFile(t, home, "state-dir: /from-user\n")

	c, _, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.StateDir != "/from-user" {
		t.Fatalf("StateDir = %q, want /from-user", c.StateDir)
	}

	// An explicit --config file named .fcvm.yaml in the current directory is
	// still an explicit file, not a Directory config.
	explicit := writeConfigFile(t, other, "state-dir: /from-explicit\n")
	c, _, err = Resolve(viper.New(), ResolveInput{ConfigFile: explicit, Cwd: other, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.StateDir != "/from-explicit" {
		t.Fatalf("StateDir = %q, want /from-explicit", c.StateDir)
	}
}

func TestResolveStateDirFromFlagAndEnvWorksWithDirectoryConfig(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	writeConfigFile(t, home, "state-dir: /from-user\n")
	writeConfigFile(t, cwd, "vcpu-count: 8\n")

	flags := pflag.NewFlagSet("fcvm", pflag.ContinueOnError)
	flags.String("state-dir", "", "")
	if err := flags.Parse([]string{"--state-dir", "/from-flag"}); err != nil {
		t.Fatal(err)
	}
	v := viper.New()
	if err := v.BindPFlag("state-dir", flags.Lookup("state-dir")); err != nil {
		t.Fatal(err)
	}
	c, _, err := Resolve(v, ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.StateDir != "/from-flag" {
		t.Fatalf("StateDir = %q, want /from-flag", c.StateDir)
	}

	v = viper.New()
	v.SetEnvPrefix("FCVM")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()
	t.Setenv("FCVM_STATE_DIR", "/from-env")
	c, _, err = Resolve(v, ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.StateDir != "/from-env" {
		t.Fatalf("StateDir = %q, want /from-env", c.StateDir)
	}
}

func TestResolveDirectoryConfigMayPinKernelRootfsAndBinaries(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	writeConfigFile(t, cwd, "kernel: /k/vmlinux\nrootfs: /r/rootfs.ext4\nfirecracker-bin: /b/firecracker\njailer-bin: /b/jailer\nguest-agent-bin: /b/agent\n")

	c, _, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: home, UID: os.Getuid()})
	if err != nil {
		t.Fatal(err)
	}
	if c.Kernel != "/k/vmlinux" || c.Rootfs != "/r/rootfs.ext4" || c.FirecrackerBin != "/b/firecracker" || c.JailerBin != "/b/jailer" || c.GuestAgentBin != "/b/agent" {
		t.Fatalf("Directory config values not applied: %+v", c)
	}
}
