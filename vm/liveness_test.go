package vm

import (
	"os/exec"
	"testing"
)

// IsRunning must distinguish "our process" from "some process". The recycled
// PID case is the one that matters: fcvm runs as root, and signalling a PID it
// no longer owns kills an unrelated process.
func TestIsRunningIdentifiesTheProcess(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	start, err := procStartTime(pid)
	if err != nil {
		t.Fatalf("procStartTime: %v", err)
	}

	live := &State{ID: "live", PID: pid, PIDStart: start}
	if !live.IsRunning() {
		t.Fatal("a running process with a matching start time should be running")
	}

	// Same live PID, but recorded against a different process: this is what a
	// recycled PID looks like, and it must not be reported as ours.
	stale := &State{ID: "stale", PID: pid, PIDStart: start + 1}
	if stale.IsRunning() {
		t.Fatal("a PID whose start time does not match must not be reported as running")
	}

	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	if live.IsRunning() {
		t.Fatal("an exited process should not be running")
	}
}

func TestIsRunningZeroPID(t *testing.T) {
	if (&State{PID: 0}).IsRunning() {
		t.Fatal("pid 0 is not a running VM")
	}
	if (*State)(nil).IsRunning() {
		t.Fatal("nil state is not running")
	}
}
