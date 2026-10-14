package cmd

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/pminnebach/fcvm/config"
	"github.com/pminnebach/fcvm/vm"
)

// runCLI executes the real command tree against a temp state directory.
func runCLI(t *testing.T, stateDir string, args ...string) (string, error) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("state-dir", stateDir)
	viper.Set("enable-experimental", true)
	// Flags and args persist on the global command tree between runs.
	t.Cleanup(func() {
		for _, name := range []string{"mount"} {
			if f := startCmd.Flags().Lookup(name); f != nil {
				if sv, ok := f.Value.(pflag.SliceValue); ok {
					_ = sv.Replace(nil)
				}
				f.Changed = false
			}
		}
		if f := startCmd.Flags().Lookup("env"); f != nil {
			_ = f.Value.Set("")
			f.Changed = false
		}
		if f := deleteCmd.Flags().Lookup("force"); f != nil {
			_ = f.Value.Set("false")
			f.Changed = false
		}
	})
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

func seedStopped(t *testing.T, dir, id string, cni bool) {
	t.Helper()
	cfg := config.Default()
	vc := &vm.VMConfig{ID: id, Config: cfg, TapDev: "fcvm-tap-0", GuestIP: "172.16.0.2"}
	if cni {
		vc.Config.Network.CNINetwork = "fcvm"
		vc.GuestIP = ""
	}
	if err := vm.SaveVMConfig(dir, vc); err != nil {
		t.Fatal(err)
	}
	if err := vm.SaveStopRecord(dir, id, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func TestStartExistingVMRejectsMountAndEnvFlags(t *testing.T) {
	dir := t.TempDir()
	seedStopped(t, dir, "web", false)

	for _, args := range [][]string{
		{"start", "web", "--mount", "/a:/b"},
		{"start", "web", "--env", "K=v"},
	} {
		_, err := runCLI(t, dir, args...)
		if err == nil {
			t.Fatalf("%v: expected an error", args)
		}
		if !strings.Contains(err.Error(), "fcvm delete web") {
			t.Fatalf("%v: error should say how to change a VM's shape, got %v", args, err)
		}
	}
}

func TestRestartUnknownVMIsAnError(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("the manager checks for root before it looks the VM up")
	}
	_, err := runCLI(t, t.TempDir(), "restart", "ghost")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "no such VM") {
		t.Fatalf("want a no-such-VM error, got %v", err)
	}
}

func TestRestartTakesNoShapeFlags(t *testing.T) {
	if restartCmd.Flags().Lookup("mount") != nil || restartCmd.Flags().Lookup("env") != nil {
		t.Fatal("restart reuses the saved configuration and must not accept --mount or --env")
	}
}

func TestDeleteFlagsAndUnknownVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("the manager checks for root before it looks the VM up")
	}
	if deleteCmd.Flags().Lookup("force") == nil {
		t.Fatal("delete needs --force")
	}
	_, err := runCLI(t, t.TempDir(), "delete", "ghost")
	if err == nil {
		t.Fatal("expected an error for an unknown VM")
	}
}

func TestExecOnStoppedVMExplainsAndSuggestsStart(t *testing.T) {
	dir := t.TempDir()
	seedStopped(t, dir, "web", false)

	for _, args := range [][]string{
		{"exec", "web", "--", "true"},
		{"shell", "web"},
	} {
		_, err := runCLI(t, dir, args...)
		if err == nil || !strings.Contains(err.Error(), "fcvm start web") {
			t.Fatalf("%v: want a hint to start the VM, got %v", args, err)
		}
	}
}

func TestRenderList(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	infos := []vm.Info{
		{ID: "up", Status: vm.StatusRunning, State: &vm.State{ID: "up", PID: 42, GuestIP: "172.16.0.2", StartedAt: now.Add(-90 * time.Second)}},
		{ID: "tap", Status: vm.StatusStopped, StoppedAt: now.Add(-2 * time.Hour),
			Config: &vm.VMConfig{ID: "tap", GuestIP: "172.16.0.6"}},
		{ID: "cni", Status: vm.StatusStopped, StoppedAt: now.Add(-time.Minute),
			Config: &vm.VMConfig{ID: "cni", GuestIP: "10.0.0.9", Config: config.Config{Network: config.NetworkConfig{CNINetwork: "fcvm"}}}},
		{ID: "dead", Status: vm.StatusCrashed, State: &vm.State{ID: "dead", PID: 7, GuestIP: "172.16.0.10"},
			Config: &vm.VMConfig{ID: "dead", GuestIP: "172.16.0.10"}},
		{ID: "old", Status: vm.StatusLegacy, State: &vm.State{ID: "old", PID: 9, GuestIP: "172.16.0.14"}},
	}
	var buf bytes.Buffer
	if err := renderList(&buf, infos, now); err != nil {
		t.Fatal(err)
	}
	rows := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n")[1:] {
		f := strings.Fields(line)
		rows[f[0]] = f
	}
	want := map[string][]string{
		"up":   {"up", "running", "172.16.0.2", "42", "1m30s", "-"},
		"tap":  {"tap", "stopped", "172.16.0.6", "-", "-", "2h0m0s", "ago"},
		"cni":  {"cni", "stopped", "-", "-", "-", "1m0s", "ago"},
		"dead": {"dead", "crashed", "172.16.0.10", "-", "-", "-"},
		"old":  {"old", "legacy", "172.16.0.14", "-", "-", "-"},
	}
	for id, w := range want {
		if strings.Join(rows[id], " ") != strings.Join(w, " ") {
			t.Errorf("%s: got %v, want %v", id, rows[id], w)
		}
	}
	if !strings.HasPrefix(buf.String(), "ID") || !strings.Contains(strings.SplitN(buf.String(), "\n", 2)[0], "STOPPED") {
		t.Errorf("header = %q", strings.SplitN(buf.String(), "\n", 2)[0])
	}
}
