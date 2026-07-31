package guest

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ssh joins its trailing arguments with spaces and hands the result to a
// remote shell, so unquoted argv is re-split there: `exec vm -- touch "my
// file"` used to create two files.
func TestRemoteCommandPreservesWordBoundaries(t *testing.T) {
	cases := [][]string{
		{"touch", "my file"},
		{"echo", "a  b", "$HOME", "`id`", `quote"inside`, "semi;colon"},
		{"sh", "-c", "echo hello world"},
	}
	for _, args := range cases {
		remote := RemoteCommand(args)
		// Let a real shell re-split it the way the remote side will.
		out, err := exec.Command("sh", "-c", "for a in "+remote+`; do printf '%s\n' "$a"; done`).Output()
		if err != nil {
			t.Fatalf("re-parsing %q: %v", remote, err)
		}
		got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
		if len(got) != len(args) {
			t.Fatalf("argv %q round-tripped as %q via %s", args, got, remote)
		}
		for i := range args {
			if got[i] != args[i] {
				t.Errorf("arg %d = %q, want %q (remote: %s)", i, got[i], args[i], remote)
			}
		}
	}
}

func TestShellQuote(t *testing.T) {
	if got := ShellQuote("it's"); got != `'it'\''s'` {
		t.Fatalf("ShellQuote = %s", got)
	}
}

func TestLoadOrCreateKeyRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys", "id_ed25519")

	created, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.PublicKey, "ssh-ed25519 ") {
		t.Fatalf("public key = %q", created.PublicKey)
	}

	// A second call must reuse the key, not mint a new one: the first key is
	// already baked into every rootfs built so far.
	loaded, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PublicKey != created.PublicKey {
		t.Fatal("LoadOrCreateKey regenerated an existing key")
	}
}
