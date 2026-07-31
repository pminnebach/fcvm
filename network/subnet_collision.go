package network

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
)

// Stock TAP bases from config defaults. ResolveTapAddrs may rebase only when
// the configured bases equal these; any other base is sticky and hard-errors.
const (
	DefaultTapIP   = "172.16.0.1"
	DefaultGuestIP = "172.16.0.2"

	altTapIP   = "10.200.0.1"
	altGuestIP = "10.200.0.2"
)

type localIPv4 struct {
	ifname string
	ip     net.IP
	ipnet  *net.IPNet
}

type hostIPv4 struct {
	locals []localIPv4
	routes []*net.IPNet
}

// AssertSubnetFree reports an error when the proposed TAP /30 overlaps a
// host-local IPv4 address or on-link IPv4 route.
func AssertSubnetFree(tapIP, guestIP string) error {
	host, err := loadHostIPv4()
	if err != nil {
		return err
	}
	return subnetConflict(tapIP, guestIP, host)
}

// ResolveTapAddrs derives tap/guest IPs for index and ensures the /30 is free
// on the host. Stock default bases may rebase onto 10.200.*; any other base
// hard-errors on collision so explicit config stays sticky.
func ResolveTapAddrs(baseTap, baseGuest string, index int) (tapIP, guestIP string, rebased bool, err error) {
	host, err := loadHostIPv4()
	if err != nil {
		return "", "", false, err
	}
	return resolveTapAddrs(host, baseTap, baseGuest, index)
}

func resolveTapAddrs(host hostIPv4, baseTap, baseGuest string, index int) (tapIP, guestIP string, rebased bool, err error) {
	tapIP, guestIP, err = SubnetForIndex(baseTap, baseGuest, index)
	if err != nil {
		return "", "", false, err
	}
	if err := subnetConflict(tapIP, guestIP, host); err == nil {
		return tapIP, guestIP, false, nil
	} else if !isDefaultBase(baseTap, baseGuest) {
		return "", "", false, fmt.Errorf("%w; set network.tap-ip / network.guest-ip to a non-overlapping base", err)
	}

	// Default bases collided: try 10.200.{index}.0/30, then walk third octet.
	for octet := 0; octet < 256; octet++ {
		n := (index + octet) % 256
		candTap, candGuest, err := SubnetForIndex(altTapIP, altGuestIP, n)
		if err != nil {
			continue
		}
		if err := subnetConflict(candTap, candGuest, host); err != nil {
			continue
		}
		return candTap, candGuest, true, nil
	}
	return "", "", false, fmt.Errorf("guest subnet for defaults collides with the host and no free /30 found in 10.200.0.0/16; set network.tap-ip / network.guest-ip")
}

func isDefaultBase(tap, guest string) bool {
	return tap == DefaultTapIP && guest == DefaultGuestIP
}

func loadHostIPv4() (hostIPv4, error) {
	addrOut, err := exec.Command("ip", "-j", "addr").Output()
	if err != nil {
		return hostIPv4{}, fmt.Errorf("ip addr: %w", err)
	}
	routeOut, err := exec.Command("ip", "-j", "route").Output()
	if err != nil {
		return hostIPv4{}, fmt.Errorf("ip route: %w", err)
	}
	locals, err := parseLocalIPv4s(addrOut)
	if err != nil {
		return hostIPv4{}, err
	}
	routes, err := parseOnLinkRoutes(routeOut)
	if err != nil {
		return hostIPv4{}, err
	}
	return hostIPv4{locals: locals, routes: routes}, nil
}

func parseLocalIPv4s(jsonOut []byte) ([]localIPv4, error) {
	var ifaces []struct {
		Ifname   string `json:"ifname"`
		AddrInfo []struct {
			Family    string `json:"family"`
			Local     string `json:"local"`
			PrefixLen int    `json:"prefixlen"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal(jsonOut, &ifaces); err != nil {
		return nil, fmt.Errorf("parse ip addr json: %w", err)
	}
	var out []localIPv4
	for _, iface := range ifaces {
		for _, a := range iface.AddrInfo {
			if a.Family != "inet" {
				continue
			}
			ip := net.ParseIP(a.Local).To4()
			if ip == nil {
				continue
			}
			mask := net.CIDRMask(a.PrefixLen, 32)
			out = append(out, localIPv4{
				ifname: iface.Ifname,
				ip:     ip,
				ipnet:  &net.IPNet{IP: ip.Mask(mask), Mask: mask},
			})
		}
	}
	return out, nil
}

func parseOnLinkRoutes(jsonOut []byte) ([]*net.IPNet, error) {
	var routes []struct {
		Dst   string `json:"dst"`
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal(jsonOut, &routes); err != nil {
		return nil, fmt.Errorf("parse ip route json: %w", err)
	}
	var out []*net.IPNet
	for _, r := range routes {
		if r.Scope != "link" || r.Dst == "" || r.Dst == "default" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(r.Dst)
		if err != nil {
			// Host routes sometimes appear as a bare address.
			ip := net.ParseIP(r.Dst).To4()
			if ip == nil {
				continue
			}
			mask := net.CIDRMask(32, 32)
			ipnet = &net.IPNet{IP: ip, Mask: mask}
		}
		if ipnet.IP.To4() == nil {
			continue
		}
		out = append(out, ipnet)
	}
	return out, nil
}

func subnetConflict(tapIP, guestIP string, host hostIPv4) error {
	subnet, err := GuestSubnet(tapIP)
	if err != nil {
		return err
	}
	_, proposed, err := net.ParseCIDR(subnet)
	if err != nil {
		return err
	}
	tap := net.ParseIP(tapIP).To4()
	guest := net.ParseIP(guestIP).To4()
	if tap == nil || guest == nil {
		return fmt.Errorf("tap-ip %q / guest-ip %q must be IPv4", tapIP, guestIP)
	}

	for _, loc := range host.locals {
		if loc.ip.Equal(tap) || loc.ip.Equal(guest) {
			return fmt.Errorf("guest subnet %s conflicts with host address %s on %s",
				subnet, loc.ip, loc.ifname)
		}
		if netsOverlap(proposed, loc.ipnet) || proposed.Contains(loc.ip) {
			return fmt.Errorf("guest subnet %s overlaps host address %s/%d on %s",
				subnet, loc.ip, ones(loc.ipnet.Mask), loc.ifname)
		}
	}
	for _, route := range host.routes {
		if netsOverlap(proposed, route) {
			return fmt.Errorf("guest subnet %s overlaps host on-link route %s", subnet, route)
		}
	}
	return nil
}

func netsOverlap(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

func ones(mask net.IPMask) int {
	n, _ := mask.Size()
	return n
}
