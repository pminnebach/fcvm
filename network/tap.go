package network

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

func SetupTap(tapDev, tapIP, guestIP string) error {
	mask := "/30"
	if err := run("ip", "link", "del", tapDev); err != nil && !strings.Contains(err.Error(), "Cannot find device") {
		// ignore missing tap
	}
	if err := run("ip", "tuntap", "add", "dev", tapDev, "mode", "tap"); err != nil {
		return fmt.Errorf("create tap %s: %w", tapDev, err)
	}
	if err := run("ip", "addr", "add", tapIP+mask, "dev", tapDev); err != nil {
		return fmt.Errorf("assign tap ip: %w", err)
	}
	if err := run("ip", "link", "set", "dev", tapDev, "up"); err != nil {
		return fmt.Errorf("bring tap up: %w", err)
	}
	_ = run("sh", "-c", fmt.Sprintf("echo 1 > /proc/sys/net/ipv4/conf/%s/proxy_arp", tapDev))
	if err := run("sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward"); err != nil {
		return fmt.Errorf("enable ip_forward: %w", err)
	}
	if err := run("iptables", "-P", "FORWARD", "ACCEPT"); err != nil {
		return fmt.Errorf("iptables forward: %w", err)
	}
	hostIface, err := defaultIface()
	if err != nil {
		return err
	}
	_ = run("iptables", "-t", "nat", "-D", "POSTROUTING", "-o", hostIface, "-j", "MASQUERADE")
	if err := run("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", hostIface, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("iptables masquerade: %w", err)
	}
	_ = guestIP // guest configures default route via fcnet or boot hook
	return nil
}

func TeardownTap(tapDev string) {
	_ = run("ip", "link", "del", tapDev)
}

func defaultIface() (string, error) {
	out, err := exec.Command("ip", "-j", "route", "list", "default").Output()
	if err != nil {
		return "", fmt.Errorf("default route: %w", err)
	}
	// ponytail: naive string parse; upgrade path: json.Unmarshal ip -j output
	s := string(out)
	i := strings.Index(s, `"dev":`)
	if i < 0 {
		return "eth0", nil
	}
	rest := s[i+6:]
	rest = strings.TrimPrefix(rest, `"`)
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "eth0", nil
	}
	return rest[:end], nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %s: %w", name, args, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// GuestMAC derives MAC from guest IP (Firecracker fcnet pattern: 06:00:a.b.c.d).
func GuestMAC(guestIP string) string {
	ip := net.ParseIP(guestIP).To4()
	if ip == nil {
		return "06:00:AC:10:00:02"
	}
	return fmt.Sprintf("06:00:%02X:%02X:%02X:%02X", ip[0], ip[1], ip[2], ip[3])
}

// SubnetForIndex returns tap/guest IPs for VM index (0-based).
func SubnetForIndex(baseTap, baseGuest string, index int) (tapIP, guestIP string) {
	tap := net.ParseIP(baseTap).To4()
	guest := net.ParseIP(baseGuest).To4()
	if tap == nil || guest == nil {
		return baseTap, baseGuest
	}
	// shift third octet by index
	tap[2] = byte(int(tap[2]) + index)
	guest[2] = byte(int(guest[2]) + index)
	return tap.String(), guest.String()
}
