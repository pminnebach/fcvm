package rootfs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Env values are written by the host and sourced by the guest shell. Building
// those lines by concatenation corrupts any value containing a quote, and
// executes any value containing a command substitution.
func TestRenderEnvSurvivesShellMetacharacters(t *testing.T) {
	values := map[string]string{
		"QUOTES":  `he said "hi" and 'bye'`,
		"DOLLAR":  `$(touch /tmp/fcvm-pwned) ${HOME} $USER`,
		"BACKTIK": "a`b`c",
		"SLASH":   `a\b\\c`,
		"SPACES":  "  leading and trailing  ",
		"EQUALS":  "k=v=w",
	}
	rendered := RenderEnv(values)

	dir := t.TempDir()
	envFile := filepath.Join(dir, "env")
	if err := os.WriteFile(envFile, []byte(rendered), 0o644); err != nil {
		t.Fatal(err)
	}
	canary := filepath.Join(dir, "fcvm-pwned")

	for key, want := range values {
		out, err := exec.Command("sh", "-c",
			". "+envFile+`; printf %s "$`+key+`"`).Output()
		if err != nil {
			t.Fatalf("sourcing env for %s: %v\nfile:\n%s", key, err, rendered)
		}
		if string(out) != want {
			t.Errorf("%s = %q, want %q", key, out, want)
		}
	}
	if _, err := os.Stat(canary); err == nil {
		t.Fatal("a command substitution in an env value executed")
	}
}

func TestShellQuote(t *testing.T) {
	if got := ShellQuote("plain"); got != "'plain'" {
		t.Fatalf("ShellQuote(plain) = %s", got)
	}
	if got := ShellQuote("it's"); got != `'it'\''s'` {
		t.Fatalf("ShellQuote(it's) = %s", got)
	}
}

func TestRenderMountsIsTabSeparated(t *testing.T) {
	out := RenderMounts([]MountRecord{
		{Method: "nfs", Source: "172.16.0.1:/exports/vm-0/share", Guest: "/data"},
		{Method: "block", Source: "/dev/vdb", Guest: "/scratch"},
	})
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines: %q", len(lines), out)
	}
	for _, l := range lines {
		if len(strings.Split(l, "\t")) != 3 {
			t.Fatalf("line is not three tab-separated fields: %q", l)
		}
	}
}

// The dead fcvm-mounts.sh stub must not come back.
func TestInjectHooksOmitsDeadScript(t *testing.T) {
	dir := t.TempDir()
	if err := InjectHooks(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "usr/local/bin/fcvm-mounts.sh")); err == nil {
		t.Fatal("fcvm-mounts.sh is injected but nothing calls it")
	}
}
