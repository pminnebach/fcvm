package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// brokenUserConfig points HOME at a temp dir whose User config holds a value
// that cannot be decoded, and runs from an empty directory. This is how a
// broken config reaches tab-completion in practice: cobra's __complete
// disables flag parsing, so --config is never seen there.
func brokenUserConfig(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".fcvm.yaml"), []byte("vcpu-count: \"not-a-number\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")
	t.Chdir(t.TempDir())
}

func TestCompletionIgnoresBrokenConfig(t *testing.T) {
	brokenUserConfig(t)
	// Output is not checked: cobra builds the completion subcommands once and
	// keeps the writer of the first Execute, so runCLI's buffer may not get it.
	if _, err := runCLI(t, t.TempDir(), "completion", "bash"); err != nil {
		t.Fatalf("completion bash failed on a broken config: %v", err)
	}
}

func TestCompletionRequestIgnoresBrokenConfig(t *testing.T) {
	for _, req := range []string{"__complete", "__completeNoDesc"} {
		t.Run(req, func(t *testing.T) {
			brokenUserConfig(t)
			out, err := runCLI(t, t.TempDir(), req, "st")
			if err != nil {
				t.Fatalf("%s failed on a broken config: %v", req, err)
			}
			if !strings.Contains(out, "start") {
				t.Fatalf("%s st: want start among candidates, got %q", req, out)
			}
		})
	}
}

func TestHelpLikeCommandsIgnoreBrokenConfig(t *testing.T) {
	for _, args := range [][]string{{"version"}, {"help"}, {"help", "list"}, {"experimental"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			brokenUserConfig(t)
			if _, err := runCLI(t, t.TempDir(), args...); err != nil {
				t.Fatalf("%v failed on a broken config: %v", args, err)
			}
		})
	}
}

func TestOrdinaryCommandFailsOnBrokenConfig(t *testing.T) {
	brokenUserConfig(t)
	_, err := runCLI(t, t.TempDir(), "list")
	if err == nil {
		t.Fatal("list succeeded on a broken config")
	}
	if !strings.Contains(err.Error(), "vcpu-count") {
		t.Fatalf("want an error about vcpu-count, got %v", err)
	}
}
