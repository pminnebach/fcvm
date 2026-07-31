# Host subnet collision

Refuse TAP-mode start when the derived guest `/30` overlaps host-local addresses or on-link routes, so a nested `fcvm start` cannot steal the outer guest's gateway and brick the sandbox.

## Symptom

An agent or developer runs inside a Firecracker guest that already uses fcvm's default addressing (`eth0 = 172.16.0.2/30`, gateway `172.16.0.1`, MAC `06:00:ac:10:00:02`). Then:

```bash
sudo fcvm start nested   # index 0 → assigns 172.16.0.1/30 to fcvm-tap-0
```

The outer guest's only default route is `via 172.16.0.1`. After `SetupTap`, that address is **local** on `fcvm-tap-0`, so packets meant for the outer hypervisor never leave. SSH / the agent control plane die and the sandbox looks unreachable.

## Root cause

`Manager.Start` ([vm/manager.go](../vm/manager.go)) derives `tapIP` / `guestIP` from config + index and calls `SetupTap` with no check against the host's existing addresses:

```go
tapIP, guestIP, err = network.SubnetForIndex(m.cfg.Network.TapIP, m.cfg.Network.GuestIP, index)
// ...
hostIface, err = network.DefaultIface()
// ...
if err := network.SetupTap(tapDev, tapIP, guestIP, hostIface); err != nil {
```

Defaults are `tap-ip: 172.16.0.1`, `guest-ip: 172.16.0.2` ([docs/network.md](../docs/network.md)). On a bare metal or cloud host that is fine. Inside an fcvm-shaped guest, those are exactly the host's eth0 addresses, so `ip addr add 172.16.0.1/30 dev fcvm-tap-0` is a silent takeover of the control plane, not a start failure.

Index allocation and `SetupTap`'s "device already exists" check only protect against other **fcvm** TAPs. They do not see eth0.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Behavior | **Hard error** before `SetupTap`; do not auto-pick another range |
| Why not auto-pick | Silent IP change breaks operators who rely on config/index; the VM index already owns the third octet for TAP name and jailer uid |
| When | TAP mode only, after `SubnetForIndex` / `DefaultIface`, **before** `EnableIPForward` / `SetupTap` |
| What counts as conflict | Proposed `/30` overlaps any host-local IPv4 **or** any on-link (connected) IPv4 route; also if `tapIP` or `guestIP` equals an existing local address |
| How to observe host | `ip -j addr` and `ip -j route` (same JSON style as `DefaultIface`); parse in pure helpers for unit tests |
| Error text | Name the conflicting host address/route and the proposed subnet; tell the operator to set `network.tap-ip` / `network.guest-ip` (or flags) to a non-overlapping base |
| CNI | Out of scope — CNI owns addressing |
| Existing fcvm TAPs | Overlap with addresses on `fcvm-tap-*` is already blocked by index + `SetupTap` existence check; this guard is about **non-fcvm** host interfaces/routes (especially eth0) |

## Fix

1. Add `network.AssertSubnetFree(tapIP, guestIP string) error` in [network/tap.go](../network/tap.go) (or a small adjacent file). Compute `GuestSubnet(tapIP)`, load host addresses and routes, reject overlaps.
2. Split observation from policy: helpers that parse `ip -j addr` / `ip -j route` JSON into local IPv4s and on-link prefixes, then a pure overlap check against the proposed `/30`. Production code shells out; tests feed fixture bytes.
3. Call `AssertSubnetFree` from `Manager.Start` on the TAP path after IPs and `hostIface` are known, before `EnableIPForward` / `SetupTap`, so a collision never mutates iptables or creates a TAP.
4. On conflict, return an error that names the conflicting host address or route, the proposed subnet, and points at `network.tap-ip` / `network.guest-ip`.
5. Document in [docs/network.md](../docs/network.md) under Static TAP: start fails if the `/30` collides with the host; nested hosts must override the default base (for example `10.200.0.1` / `10.200.0.2`).

## Code touch list

| Area | Change |
|------|--------|
| [network/tap.go](../network/tap.go) (or adjacent) | `AssertSubnetFree`; JSON parsers for `ip -j addr` / on-link routes; overlap helpers |
| [vm/manager.go](../vm/manager.go) | Call `AssertSubnetFree` on the TAP path before host network mutation |
| [docs/network.md](../docs/network.md) | Note the host-collision refuse and nested override |

## Check to leave behind

Unit tests in `network/` with fixture JSON (no root, no live `ip`):

- Nested case: host has `172.16.0.2/30` on eth0 (and/or on-link `172.16.0.0/30`) vs proposed `172.16.0.0/30` → error naming the conflict.
- Disjoint case: host on `172.16.0.0/30`, proposed `10.200.0.0/30` → ok.
- Empty host addrs/routes → ok.
- Exact equality: proposed `tapIP` or `guestIP` already a local address → error.

## Non-goals

- No auto-selection of a free `/30`.
- Do not change the default `172.16.0.0/30` base for ordinary hosts.
- Do not scan or skip VM indices to dodge host collisions.
- Do not touch the CNI path.
- Do not try to detect "we are nested in Firecracker" as a special case; address overlap is the only signal.

## Success criteria

- `fcvm start` with defaults inside a `172.16.0.2/30` guest fails before creating `fcvm-tap-0` or changing iptables / `ip_forward`, and the outer eth0 route still works.
- The same start with a non-overlapping `--tap-ip` / `--guest-ip` (or config) proceeds as today.
- CNI-mode start is unchanged.
