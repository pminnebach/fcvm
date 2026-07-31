package network

import (
	"net"
	"strings"
	"testing"
)

const nestedAddrJSON = `[
  {"ifname":"lo","addr_info":[{"family":"inet","local":"127.0.0.1","prefixlen":8}]},
  {"ifname":"eth0","addr_info":[{"family":"inet","local":"172.16.0.2","prefixlen":30}]}
]`

const nestedRouteJSON = `[
  {"dst":"default","gateway":"172.16.0.1","dev":"eth0"},
  {"dst":"172.16.0.0/30","dev":"eth0","protocol":"kernel","scope":"link","prefsrc":"172.16.0.2"}
]`

func TestParseLocalIPv4s(t *testing.T) {
	locals, err := parseLocalIPv4s([]byte(nestedAddrJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(locals) != 2 {
		t.Fatalf("got %d locals, want 2", len(locals))
	}
	if locals[1].ifname != "eth0" || !locals[1].ip.Equal(net.ParseIP("172.16.0.2")) {
		t.Fatalf("unexpected eth0 local: %+v", locals[1])
	}
	if locals[1].ipnet.String() != "172.16.0.0/30" {
		t.Fatalf("eth0 net = %s, want 172.16.0.0/30", locals[1].ipnet)
	}
}

func TestParseOnLinkRoutes(t *testing.T) {
	routes, err := parseOnLinkRoutes([]byte(nestedRouteJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || routes[0].String() != "172.16.0.0/30" {
		t.Fatalf("routes = %v, want [172.16.0.0/30]", routes)
	}
}

func TestSubnetConflictNested(t *testing.T) {
	host := mustHost(t, nestedAddrJSON, nestedRouteJSON)
	err := subnetConflict("172.16.0.1", "172.16.0.2", host)
	if err == nil {
		t.Fatal("expected conflict with nested eth0")
	}
	if !strings.Contains(err.Error(), "172.16.0.0/30") {
		t.Fatalf("error should name proposed subnet: %v", err)
	}
}

func TestSubnetConflictDisjoint(t *testing.T) {
	host := mustHost(t, nestedAddrJSON, nestedRouteJSON)
	if err := subnetConflict("10.200.0.1", "10.200.0.2", host); err != nil {
		t.Fatalf("disjoint subnet should be free: %v", err)
	}
}

func TestSubnetConflictEmptyHost(t *testing.T) {
	if err := subnetConflict("172.16.0.1", "172.16.0.2", hostIPv4{}); err != nil {
		t.Fatalf("empty host should be free: %v", err)
	}
}

func TestSubnetConflictExactLocalEquality(t *testing.T) {
	host := hostIPv4{
		locals: []localIPv4{{
			ifname: "eth0",
			ip:     net.ParseIP("10.0.0.1").To4(),
			ipnet:  mustCIDR(t, "10.0.0.1/32"),
		}},
	}
	err := subnetConflict("10.0.0.1", "10.0.0.2", host)
	if err == nil {
		t.Fatal("expected conflict when tapIP equals a local address")
	}
	if !strings.Contains(err.Error(), "10.0.0.1") {
		t.Fatalf("error should name the host address: %v", err)
	}
}

func TestResolveTapAddrsRebasesDefaults(t *testing.T) {
	host := mustHost(t, nestedAddrJSON, nestedRouteJSON)
	tap, guest, rebased, err := resolveTapAddrs(host, DefaultTapIP, DefaultGuestIP, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !rebased {
		t.Fatal("expected rebase for colliding defaults")
	}
	if tap != "10.200.0.1" || guest != "10.200.0.2" {
		t.Fatalf("got %s/%s, want 10.200.0.1/10.200.0.2", tap, guest)
	}
}

func TestResolveTapAddrsHardErrorOnExplicit(t *testing.T) {
	host := mustHost(t, `[
	  {"ifname":"eth0","addr_info":[{"family":"inet","local":"192.168.1.2","prefixlen":30}]}
	]`, `[
	  {"dst":"192.168.1.0/30","dev":"eth0","scope":"link"}
	]`)
	_, _, _, err := resolveTapAddrs(host, "192.168.1.1", "192.168.1.2", 0)
	if err == nil {
		t.Fatal("expected hard error for explicit colliding base")
	}
	if !strings.Contains(err.Error(), "network.tap-ip") {
		t.Fatalf("error should point at config keys: %v", err)
	}
}

func TestResolveTapAddrsNoRebaseWhenFree(t *testing.T) {
	host := mustHost(t, nestedAddrJSON, nestedRouteJSON)
	tap, guest, rebased, err := resolveTapAddrs(host, "10.200.0.1", "10.200.0.2", 0)
	if err != nil {
		t.Fatal(err)
	}
	if rebased {
		t.Fatal("disjoint explicit base should not rebase")
	}
	if tap != "10.200.0.1" || guest != "10.200.0.2" {
		t.Fatalf("got %s/%s", tap, guest)
	}
}

func TestResolveTapAddrsWalksWhenAltBusy(t *testing.T) {
	// Nested 172.16 plus 10.200.0.0/30 already taken → walk to .1
	host := mustHost(t, `[
	  {"ifname":"eth0","addr_info":[{"family":"inet","local":"172.16.0.2","prefixlen":30}]},
	  {"ifname":"fcvm-tap-9","addr_info":[{"family":"inet","local":"10.200.0.1","prefixlen":30}]}
	]`, `[
	  {"dst":"172.16.0.0/30","dev":"eth0","scope":"link"},
	  {"dst":"10.200.0.0/30","dev":"fcvm-tap-9","scope":"link"}
	]`)
	tap, guest, rebased, err := resolveTapAddrs(host, DefaultTapIP, DefaultGuestIP, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !rebased {
		t.Fatal("expected rebase")
	}
	if tap != "10.200.1.1" || guest != "10.200.1.2" {
		t.Fatalf("got %s/%s, want 10.200.1.1/10.200.1.2", tap, guest)
	}
}

func mustHost(t *testing.T, addrJSON, routeJSON string) hostIPv4 {
	t.Helper()
	locals, err := parseLocalIPv4s([]byte(addrJSON))
	if err != nil {
		t.Fatal(err)
	}
	routes, err := parseOnLinkRoutes([]byte(routeJSON))
	if err != nil {
		t.Fatal(err)
	}
	return hostIPv4{locals: locals, routes: routes}
}

func mustCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatal(err)
	}
	return ipnet
}
