package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveGuestAgentConfigured(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fcvm-guest-agent")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := Default()
	c.GuestAgentBin = bin
	got, err := ResolveGuestAgent(c)
	if err != nil {
		t.Fatal(err)
	}
	if got != bin {
		t.Fatalf("got %q", got)
	}
}

func TestResolveGuestAgentMissing(t *testing.T) {
	c := Default()
	c.GuestAgentBin = filepath.Join(t.TempDir(), "missing")
	if _, err := ResolveGuestAgent(c); err == nil {
		t.Fatal("expected error")
	}
}
