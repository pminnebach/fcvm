package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/config"
)

func TestExperimentalReasonsDefaultEmpty(t *testing.T) {
	cmd := &cobra.Command{Use: "start"}
	reasons := experimentalReasons(cmd, config.Default())
	if len(reasons) != 0 {
		t.Fatalf("reasons = %v, want empty", reasons)
	}
}

func TestExperimentalReasonsEnableVsock(t *testing.T) {
	c := config.Default()
	c.EnableVsock = true
	reasons := experimentalReasons(&cobra.Command{Use: "start"}, c)
	if !containsReason(reasons, "flag: --enable-vsock") {
		t.Fatalf("reasons = %v, want --enable-vsock", reasons)
	}
}

func TestExperimentalReasonsCNI(t *testing.T) {
	c := config.Default()
	c.Network.CNINetwork = "fcnet"
	reasons := experimentalReasons(&cobra.Command{Use: "start"}, c)
	if !containsReason(reasons, "flag: --cni-network") {
		t.Fatalf("reasons = %v, want --cni-network", reasons)
	}
}

func TestExperimentalReasonsVsockExecCommand(t *testing.T) {
	reasons := experimentalReasons(&cobra.Command{Use: "vsock-exec"}, config.Default())
	if !containsReason(reasons, "command: vsock-exec") {
		t.Fatalf("reasons = %v, want command: vsock-exec", reasons)
	}
}

func TestConfirmExperimentalBypass(t *testing.T) {
	c := config.Default()
	c.EnableExperimental = true
	c.EnableVsock = true
	if err := confirmExperimental(nil, c, []string{"flag: --enable-vsock"}); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmExperimentalEmptyReasons(t *testing.T) {
	if err := confirmExperimental(nil, config.Default(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmExperimentalYes(t *testing.T) {
	oldIn, oldErr := experimentalStdin, experimentalStderr
	defer func() {
		experimentalStdin, experimentalStderr = oldIn, oldErr
	}()
	experimentalStdin = strings.NewReader("y\n")
	var buf bytes.Buffer
	experimentalStderr = &buf
	if err := confirmExperimental(nil, config.Default(), []string{"flag: --enable-vsock"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "WARNING: experimental features") {
		t.Fatalf("stderr = %q", buf.String())
	}
}

func TestConfirmExperimentalNo(t *testing.T) {
	oldIn, oldErr := experimentalStdin, experimentalStderr
	defer func() {
		experimentalStdin, experimentalStderr = oldIn, oldErr
	}()
	experimentalStdin = strings.NewReader("n\n")
	experimentalStderr = ioDiscard{}
	err := confirmExperimental(nil, config.Default(), []string{"command: vsock-exec"})
	if err == nil || !strings.Contains(err.Error(), "aborted") {
		t.Fatalf("err = %v, want aborted", err)
	}
}

func TestConfirmExperimentalNonTTYFile(t *testing.T) {
	oldIn, oldErr := experimentalStdin, experimentalStderr
	defer func() {
		experimentalStdin, experimentalStderr = oldIn, oldErr
	}()
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	experimentalStdin = f
	experimentalStderr = ioDiscard{}
	err = confirmExperimental(nil, config.Default(), []string{"flag: --cni-network"})
	if err == nil || !strings.Contains(err.Error(), "--enable-experimental") {
		t.Fatalf("err = %v, want --enable-experimental hint", err)
	}
}

func TestExperimentalItemsNonEmpty(t *testing.T) {
	items := experimentalItems()
	if len(items) < 4 {
		t.Fatalf("items = %d, want at least 4", len(items))
	}
}

func containsReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
