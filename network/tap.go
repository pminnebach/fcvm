package network

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

const (
	maxIfNameLen = 15 // Linux IFNAMSIZ - 1
	tapDevPrefix = "fcvm-tap-"
	fcvmChain    = "FCVM"
)

// TapDevName returns a Linux-safe TAP interface name for the VM network index.
func TapDevName(index int) string {
	return fmt.Sprintf("%s%d", tapDevPrefix, index)
}

// IndexFromTapDev recovers the VM network index from a TAP device name.
func IndexFromTapDev(dev string) (int, bool) {
	suffix, ok := strings.CutPrefix(dev, tapDevPrefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(suffix)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// LinkExists reports whether a network interface with this name is present.
func LinkExists(dev string) bool {
	return run("ip", "link", "show", "dev", dev) == nil
}

// SetupTap creates the VM's TAP device and the forwarding and NAT rules for
// its subnet. Rules live in a dedicated FCVM chain so teardown can remove
// exactly what was added, and so the host's own FORWARD policy is left alone.
func SetupTap(tapDev, tapIP, guestIP, hostIface string) (err error) {
	if LinkExists(tapDev) {
		return fmt.Errorf("tap device %s already exists; another VM is likely using it", tapDev)
	}
	subnet, err := GuestSubnet(tapIP)
	if err != nil {
		return err
	}
	if err := run("ip", "tuntap", "add", "dev", tapDev, "mode", "tap"); err != nil {
		return fmt.Errorf("create tap %s: %w", tapDev, err)
	}
	// Undo a partial setup: SetupTap refuses to take over an existing device,
	// so a leftover half-configured tap would block every later start on this
	// index.
	defer func() {
		if err != nil {
			TeardownTap(tapDev, subnet, hostIface)
		}
	}()
	if err := run("ip", "addr", "add", tapIP+"/30", "dev", tapDev); err != nil {
		return fmt.Errorf("assign tap ip: %w", err)
	}
	if err := run("ip", "link", "set", "dev", tapDev, "up"); err != nil {
		return fmt.Errorf("bring tap up: %w", err)
	}
	_ = run("sh", "-c", fmt.Sprintf("echo 1 > /proc/sys/net/ipv4/conf/%s/proxy_arp", tapDev))

	if err := ensureFCVMChain(); err != nil {
		return err
	}
	for _, r := range vmRules(tapDev, subnet, hostIface) {
		// Delete first so a retried start does not stack duplicates.
		_ = run("iptables", r.args("-D")...)
		if err := run("iptables", r.args("-A")...); err != nil {
			return fmt.Errorf("add iptables rule: %w", err)
		}
	}
	_ = guestIP // guest configures its address via injected network config
	return nil
}

// TeardownTap removes the VM's rules and TAP device. Best effort: every step
// is independent so a partially set up VM still cleans up.
func TeardownTap(tapDev, guestSubnet, hostIface string) {
	if guestSubnet != "" && hostIface != "" {
		for _, r := range vmRules(tapDev, guestSubnet, hostIface) {
			_ = run("iptables", r.args("-D")...)
		}
	}
	_ = run("ip", "link", "del", tapDev)
}

// iptablesRule is one rule in one chain. Setup and teardown build their
// argv from the same value so they cannot drift apart and orphan a rule.
type iptablesRule struct {
	table string // empty means the default filter table
	chain string
	spec  []string
}

func (r iptablesRule) args(op string) []string {
	var out []string
	if r.table != "" {
		out = append(out, "-t", r.table)
	}
	out = append(out, op, r.chain)
	return append(out, r.spec...)
}

// vmRules returns the iptables rules owned by one VM.
func vmRules(tapDev, guestSubnet, hostIface string) []iptablesRule {
	return []iptablesRule{
		{chain: fcvmChain, spec: []string{"-i", tapDev, "-o", hostIface, "-j", "ACCEPT"}},
		{chain: fcvmChain, spec: []string{"-o", tapDev, "-i", hostIface, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"}},
		{table: "nat", chain: "POSTROUTING", spec: []string{"-s", guestSubnet, "-o", hostIface, "-j", "MASQUERADE"}},
	}
}

// ensureFCVMChain creates the FCVM chain and its FORWARD jump if missing.
// Idempotent, and it never touches the FORWARD policy.
func ensureFCVMChain() error {
	if run("iptables", "-n", "-L", fcvmChain) != nil {
		if err := run("iptables", "-N", fcvmChain); err != nil {
			return fmt.Errorf("create %s chain: %w", fcvmChain, err)
		}
	}
	if run("iptables", "-C", "FORWARD", "-j", fcvmChain) != nil {
		if err := run("iptables", "-I", "FORWARD", "1", "-j", fcvmChain); err != nil {
			return fmt.Errorf("jump to %s chain: %w", fcvmChain, err)
		}
	}
	return nil
}

// RemoveFCVMChain unhooks and deletes the chain once no VMs remain.
func RemoveFCVMChain() {
	_ = run("iptables", "-D", "FORWARD", "-j", fcvmChain)
	_ = run("iptables", "-F", fcvmChain)
	_ = run("iptables", "-X", fcvmChain)
}

// GuestSubnet returns the /30 CIDR containing the tap address.
func GuestSubnet(tapIP string) (string, error) {
	ip := net.ParseIP(tapIP).To4()
	if ip == nil {
		return "", fmt.Errorf("tap ip %q is not IPv4", tapIP)
	}
	mask := net.CIDRMask(30, 32)
	return (&net.IPNet{IP: ip.Mask(mask), Mask: mask}).String(), nil
}

// DefaultIface returns the interface holding the default route.
func DefaultIface() (string, error) {
	out, err := exec.Command("ip", "-j", "route", "list", "default").Output()
	if err != nil {
		return "", fmt.Errorf("default route: %w", err)
	}
	return parseDefaultIface(out)
}

func parseDefaultIface(jsonOut []byte) (string, error) {
	var routes []struct {
		Dev string `json:"dev"`
	}
	if err := json.Unmarshal(jsonOut, &routes); err != nil {
		return "", fmt.Errorf("parse ip route json: %w", err)
	}
	for _, r := range routes {
		if r.Dev != "" {
			return r.Dev, nil
		}
	}
	return "", fmt.Errorf("no default route; cannot determine host interface")
}

// run executes a host command. It is a variable so tests can record commands
// instead of mutating the machine they run on.
var run = execRun

func execRun(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %s: %w", name, args, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// SetRunner swaps the command runner and returns a function restoring it.
// Intended for tests; production code leaves the default in place.
func SetRunner(fn func(name string, args ...string) error) func() {
	prev := run
	run = fn
	return func() { run = prev }
}

// GuestMAC derives MAC from guest IP (Firecracker fcnet pattern: 06:00:a.b.c.d).
func GuestMAC(guestIP string) string {
	ip := net.ParseIP(guestIP).To4()
	if ip == nil {
		return "06:00:AC:10:00:02"
	}
	return fmt.Sprintf("06:00:%02X:%02X:%02X:%02X", ip[0], ip[1], ip[2], ip[3])
}

// SubnetForIndex returns tap/guest IPs for VM index (0-based) by shifting the
// third octet. It errors rather than wrapping, which would silently hand two
// VMs the same address.
func SubnetForIndex(baseTap, baseGuest string, index int) (tapIP, guestIP string, err error) {
	tap := net.ParseIP(baseTap).To4()
	guest := net.ParseIP(baseGuest).To4()
	if tap == nil || guest == nil {
		return "", "", fmt.Errorf("tap-ip %q / guest-ip %q must be IPv4", baseTap, baseGuest)
	}
	if index < 0 {
		return "", "", fmt.Errorf("vm index %d must not be negative", index)
	}
	if int(tap[2])+index > 255 || int(guest[2])+index > 255 {
		return "", "", fmt.Errorf("vm index %d exceeds the range addressable from %s/%s", index, baseTap, baseGuest)
	}
	tap[2] += byte(index)
	guest[2] += byte(index)
	return tap.String(), guest.String(), nil
}
