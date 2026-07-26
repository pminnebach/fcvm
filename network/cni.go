package network

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containernetworking/cni/libcni"
	"golang.org/x/sys/unix"
)

const (
	defaultCNIBinDir   = "/opt/cni/bin"
	defaultCNIConfDir  = "/etc/cni/conf.d"
	defaultCNICacheDir = "/var/lib/cni"
	defaultNetNSDir    = "/var/run/netns"
	DefaultCNIIfName   = "veth0"
)

// NetNSPath is the jailer/CNI netns mount path for a VM ID (SDK default layout).
func NetNSPath(vmID string) string {
	return filepath.Join(defaultNetNSDir, vmID)
}

// TeardownCNI runs CNI DEL for the VM and removes its netns mount.
// Safe to call when resources are already gone (best-effort).
func TeardownCNI(ctx context.Context, vmID, networkName string) error {
	if vmID == "" || networkName == "" {
		return nil
	}
	netNS := NetNSPath(vmID)
	cacheDir := filepath.Join(defaultCNICacheDir, vmID)

	cniPlugin := libcni.NewCNIConfigWithCacheDir([]string{defaultCNIBinDir}, cacheDir, nil)
	networkConf, err := libcni.LoadConfList(defaultCNIConfDir, networkName)
	if err != nil {
		// Still try to remove the netns even if conf is missing.
		_ = removeNetNS(netNS)
		return fmt.Errorf("load CNI network %q: %w", networkName, err)
	}
	rt := &libcni.RuntimeConf{
		ContainerID: vmID,
		NetNS:       netNS,
		IfName:      DefaultCNIIfName,
	}
	delErr := cniPlugin.DelNetworkList(ctx, networkConf, rt)
	nsErr := removeNetNS(netNS)
	if delErr != nil {
		return delErr
	}
	return nsErr
}

func removeNetNS(path string) error {
	if path == "" {
		return nil
	}
	_ = unix.Unmount(path, unix.MNT_DETACH)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove netns %q: %w", path, err)
	}
	return nil
}
