package network

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/containernetworking/cni/libcni"
)

func TestTeardownCNIEmptyNoop(t *testing.T) {
	if err := TeardownCNI(context.Background(), "", "fcnet"); err != nil {
		t.Fatal(err)
	}
	if err := TeardownCNI(context.Background(), "vm-1", ""); err != nil {
		t.Fatal(err)
	}
}

func TestNetNSPath(t *testing.T) {
	if got := NetNSPath("vm-1"); got != "/var/run/netns/vm-1" {
		t.Fatalf("got %q", got)
	}
}

// fakeCNIDeleter records DelNetworkList calls instead of invoking real CNI
// plugin binaries, so TeardownCNI's argument-building can be tested without
// root or a host CNI installation.
type fakeCNIDeleter struct {
	calls []*libcni.RuntimeConf
	err   error
}

func (f *fakeCNIDeleter) DelNetworkList(_ context.Context, _ *libcni.NetworkConfigList, rt *libcni.RuntimeConf) error {
	f.calls = append(f.calls, rt)
	return f.err
}

// withFakeCNI swaps in a fake deleter and an in-memory conflist for the
// duration of the test, restoring the real ones on cleanup.
func withFakeCNI(t *testing.T, fake cniDeleter) (gotBinDirs *[]string, gotCacheDir *string) {
	t.Helper()
	prevNew, prevLoad := newCNIDeleter, loadCNIConfList
	t.Cleanup(func() { newCNIDeleter = prevNew; loadCNIConfList = prevLoad })

	var binDirs []string
	var cacheDir string
	newCNIDeleter = func(bd []string, cd string) cniDeleter {
		binDirs, cacheDir = bd, cd
		return fake
	}
	loadCNIConfList = func(_, name string) (*libcni.NetworkConfigList, error) {
		return &libcni.NetworkConfigList{Name: name}, nil
	}
	return &binDirs, &cacheDir
}

func TestTeardownCNIInvokesDelWithCorrectArgs(t *testing.T) {
	fake := &fakeCNIDeleter{}
	binDirs, cacheDir := withFakeCNI(t, fake)

	if err := TeardownCNI(context.Background(), "vm-1", "fcnet"); err != nil {
		t.Fatalf("TeardownCNI: %v", err)
	}
	if len(fake.calls) != 1 {
		t.Fatalf("DelNetworkList calls = %d, want 1", len(fake.calls))
	}
	rt := fake.calls[0]
	if rt.ContainerID != "vm-1" || rt.NetNS != NetNSPath("vm-1") || rt.IfName != DefaultCNIIfName {
		t.Fatalf("RuntimeConf = %+v", rt)
	}
	if len(*binDirs) != 1 || (*binDirs)[0] != defaultCNIBinDir {
		t.Fatalf("binDirs = %v", *binDirs)
	}
	if *cacheDir != filepath.Join(defaultCNICacheDir, "vm-1") {
		t.Fatalf("cacheDir = %q", *cacheDir)
	}
}

func TestTeardownCNIRemovesNetNSEvenWhenDelFails(t *testing.T) {
	fake := &fakeCNIDeleter{err: errors.New("boom")}
	withFakeCNI(t, fake)

	err := TeardownCNI(context.Background(), "vm-2", "fcnet")
	if err == nil || err.Error() != "boom" {
		t.Fatalf("err = %v, want the DEL error surfaced", err)
	}
	// The netns cleanup path must still run (best-effort) even though DEL
	// failed; there's nothing at /var/run/netns/vm-2 in a unit test so this
	// just confirms DEL was attempted exactly once and the error propagated
	// rather than being swallowed.
	if len(fake.calls) != 1 {
		t.Fatalf("DelNetworkList calls = %d, want 1", len(fake.calls))
	}
}
