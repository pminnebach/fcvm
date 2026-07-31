package cmd

import (
	"testing"

	"github.com/pminnebach/fcvm/config"
)

func TestMountFlag(t *testing.T) {
	m, err := mountFlag("/data:/mnt/data:ro")
	if err != nil {
		t.Fatal(err)
	}
	if m.Host != "/data" || m.Guest != "/mnt/data" || m.Mode != "ro" {
		t.Fatalf("unexpected mount: %+v", m)
	}
	if m.Method != config.MountAuto {
		t.Fatalf("method = %q, want auto", m.Method)
	}
}

func TestMountFlagOptions(t *testing.T) {
	m, err := mountFlag("/data:/mnt:method=block,size=2G,ro")
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != config.MountBlock || m.Size != "2G" || m.Mode != "ro" {
		t.Fatalf("unexpected mount: %+v", m)
	}

	m, err = mountFlag("/data:/mnt")
	if err != nil {
		t.Fatal(err)
	}
	if m.Mode != "rw" {
		t.Fatalf("default mode = %q, want rw", m.Mode)
	}
}

// A typo in the mode used to fall through to read-write, silently handing the
// guest write access the user did not ask for.
func TestMountFlagRejectsUnknownOptions(t *testing.T) {
	for _, bad := range []string{
		"/data:/mnt:readonly",
		"/data:/mnt:method=virtiofs",
		"/data:/mnt:rx",
		"/data",
		":/mnt:ro",
		"/data::ro",
		"/data:/mnt:ro:extra",
	} {
		if m, err := mountFlag(bad); err == nil {
			t.Errorf("mountFlag(%q) = %+v, want an error", bad, m)
		}
	}
}
