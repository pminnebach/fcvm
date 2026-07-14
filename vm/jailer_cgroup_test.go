package vm

import "testing"

func TestJailerCgroupVersion(t *testing.T) {
	v := jailerCgroupVersion()
	if v != "1" && v != "2" {
		t.Fatalf("unexpected cgroup version %q", v)
	}
	t.Logf("detected cgroup version %s", v)
}
