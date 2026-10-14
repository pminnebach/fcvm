package vm

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pminnebach/fcvm/config"
)

// liveState returns a State bound to a real running process, plus a func that
// kills it, so status tests exercise the same PID-plus-start-time check as
// production.
func liveState(t *testing.T, id string) (*State, func()) {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	start, err := procStartTime(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	kill := func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	t.Cleanup(kill)
	return &State{ID: id, PID: cmd.Process.Pid, PIDStart: start}, kill
}

func deadState(id string) *State {
	return &State{ID: id, PID: 0}
}

func TestInspectStatus(t *testing.T) {
	dir := t.TempDir()
	cfg := &VMConfig{Index: 0, Config: config.Default()}

	save := func(id string, withCfg bool, st *State, stopped bool) {
		t.Helper()
		if withCfg {
			c := *cfg
			c.ID = id
			if err := SaveVMConfig(dir, &c); err != nil {
				t.Fatal(err)
			}
		}
		if st != nil {
			if err := SaveState(dir, st); err != nil {
				t.Fatal(err)
			}
		}
		if stopped {
			if err := SaveStopRecord(dir, id, time.Unix(1700000000, 0)); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.MkdirAll(filepath.Join(dir, "vms", id), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	live, _ := liveState(t, "running")
	save("running", true, live, false)
	save("crashed", true, deadState("crashed"), false)
	save("stopped", true, nil, true)
	save("legacy", false, deadState("legacy"), false)

	want := map[string]Status{
		"running": StatusRunning,
		"crashed": StatusCrashed,
		"stopped": StatusStopped,
		"legacy":  StatusLegacy,
	}
	for id, status := range want {
		info, err := Inspect(dir, id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if info.Status != status {
			t.Errorf("%s: status = %s, want %s", id, info.Status, status)
		}
	}

	info, _ := Inspect(dir, "stopped")
	if !info.StoppedAt.Equal(time.Unix(1700000000, 0)) {
		t.Errorf("StoppedAt = %v", info.StoppedAt)
	}
}

func TestInspectRecycledPIDIsNotRunning(t *testing.T) {
	dir := t.TempDir()
	live, _ := liveState(t, "recycled")
	live.PIDStart++ // same live PID, recorded against a different process
	if err := SaveVMConfig(dir, &VMConfig{ID: "recycled", Config: config.Default()}); err != nil {
		t.Fatal(err)
	}
	if err := SaveState(dir, live); err != nil {
		t.Fatal(err)
	}
	info, err := Inspect(dir, "recycled")
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != StatusCrashed {
		t.Fatalf("status = %s, want crashed", info.Status)
	}
}

func TestInspectUnknownVM(t *testing.T) {
	_, err := Inspect(t.TempDir(), "nope")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want os.ErrNotExist, got %v", err)
	}
}

func TestVMConfigIsWrittenOnce(t *testing.T) {
	dir := t.TempDir()
	c := &VMConfig{ID: "a", Index: 3, Config: config.Default()}
	c.Config.VCPUCount = 4
	c.Config.Env = map[string]string{"K": "v"}
	c.Config.Mounts = []config.MountConfig{{Host: "/h", Guest: "/g", Mode: "ro", Method: "block", Size: "10M"}}
	if err := SaveVMConfig(dir, c); err != nil {
		t.Fatal(err)
	}
	got, err := LoadVMConfig(dir, "a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Index != 3 || got.Config.VCPUCount != 4 || got.Config.Env["K"] != "v" || len(got.Config.Mounts) != 1 || got.Config.Mounts[0].Size != "10M" {
		t.Fatalf("round trip lost data: %+v", got)
	}
	if err := SaveVMConfig(dir, c); err == nil {
		t.Fatal("a configuration record must not be overwritten")
	}
}

func TestListInfosSkipsOrphanDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "vms", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveVMConfig(dir, &VMConfig{ID: "real", Config: config.Default()}); err != nil {
		t.Fatal(err)
	}
	infos, err := ListInfos(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].ID != "real" {
		t.Fatalf("infos = %+v", infos)
	}
}

func TestClaimedIndicesIncludeStoppedVMs(t *testing.T) {
	dir := t.TempDir()
	if err := SaveVMConfig(dir, &VMConfig{ID: "s", Index: 0, Config: config.Default()}); err != nil {
		t.Fatal(err)
	}
	if err := SaveStopRecord(dir, "s", time.Now()); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.StateDir = dir
	idx, err := NewManager(cfg).nextVMIndex()
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Fatalf("a stopped VM's index must stay reserved; got %d", idx)
	}
}

func TestRunningStateExplainsWhyNot(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	for _, id := range []string{"stopped", "crashed"} {
		if err := SaveVMConfig(dir, &VMConfig{ID: id, Config: cfg}); err != nil {
			t.Fatal(err)
		}
	}
	if err := SaveStopRecord(dir, "stopped", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := SaveState(dir, deadState("crashed")); err != nil {
		t.Fatal(err)
	}
	if err := SaveState(dir, deadState("legacy")); err != nil {
		t.Fatal(err)
	}
	live, _ := liveState(t, "up")
	if err := SaveVMConfig(dir, &VMConfig{ID: "up", Config: cfg}); err != nil {
		t.Fatal(err)
	}
	if err := SaveState(dir, live); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"stopped": "fcvm start stopped",
		"crashed": "crashed",
		"legacy":  "fcvm cleanup legacy",
	}
	for id, want := range cases {
		_, err := RunningState(dir, id)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want mention of %q", id, err, want)
		}
	}
	if st, err := RunningState(dir, "up"); err != nil || st.PID != live.PID {
		t.Fatalf("running VM: %v %v", st, err)
	}
	if _, err := RunningState(dir, "ghost"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unknown VM: %v", err)
	}
}
