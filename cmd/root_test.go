package cmd

import "testing"

func TestMountFlag(t *testing.T) {
	m, err := mountFlag("/data:/mnt/data:ro")
	if err != nil {
		t.Fatal(err)
	}
	if m.Host != "/data" || m.Guest != "/mnt/data" || m.Mode != "ro" {
		t.Fatalf("unexpected mount: %+v", m)
	}
}
