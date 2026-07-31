# Host subnet collision

Refuse or relocate TAP-mode addressing when the derived guest `/30` overlaps host-local addresses or on-link routes, so a nested `fcvm start` cannot steal the outer guest's gateway and brick the sandbox.

## Symptom

An agent or developer runs inside a Firecracker guest that already uses fcvm's default addressing (`eth0 = 172.16.0.2/30`, gateway `172.16.0.1`, MAC `06:00:ac:10:00:02`). Then:

```bash
sudo fcvm start nested   # index 0 → assigns 172.16.0.1/30 to fcvm-tap-0
```

The outer guest's only default route is `via 172.16.0.1`. After `SetupTap`, that address is **local** on `fcvm-tap-0`, so packets meant for the outer hypervisor never leave. SSH / the agent control plane die and the sandbox looks unreachable.

## Root cause

`Manager.Start` ([vm/manager.go](../../vm/manager.go)) derives `tapIP` / `guestIP` from config + index and calls `SetupTap` with no check against the host's existing addresses:

```go
tapIP, guestIP, err = network.SubnetForIndex(m.cfg.Network.TapIP, m.cfg.Network.GuestIP, index)
// ...
hostIface, err = network.DefaultIface()
// ...
if err := network.SetupTap(tapDev, tapIP, guestIP, hostIface); err != nil {
```

Defaults are `tap-ip: 172.16.0.1`, `guest-ip: 172.16.0.2` ([docs/network.md](../../docs/network.md)). On a bare metal or cloud host that is fine. Inside an fcvm-shaped guest, those are exactly the host's eth0 addresses, so `ip addr add 172.16.0.1/30 dev fcvm-tap-0` is a silent takeover of the control plane, not a start failure.

Index allocation and `SetupTap`'s "device already exists" check only protect against other **fcvm** TAPs. They do not see eth0.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Behavior | **Check before `SetupTap`**. If using stock default bases (`172.16.0.1` / `172.16.0.2`), **auto-rebase** to a free `/30` and log the change. If the operator set a non-default `tap-ip` / `guest-ip`, **hard error** on collision (config is sticky). |
| Why not always hard-error | Nested `fcvm start` with defaults must keep working without pre-config. |
| Why not always auto-pick | Explicit addresses are intentional; silently moving them breaks operators who set config/flags on purpose. |
| Rebase target | Keep VM **index** for TAP name and jailer uid. On default-base collision, switch base to `10.200.0.1` / `10.200.0.2` and re-derive via `SubnetForIndex` for the same index. If that `/30` still conflicts, walk the third octet within `10.200` (`.1`/`.2` roles) until free or exhausted → then hard error. |
| Default detection | Config bases equal `config` defaults (`172.16.0.1` / `172.16.0.2`). Same values from yaml/flags count as defaults (indistinguishable from viper defaults). |
| When | TAP mode only, after `SubnetForIndex` / `DefaultIface`, **before** `EnableIPForward` / `SetupTap` |
| What counts as conflict | Proposed `/30` overlaps any host-local IPv4 **or** any on-link (connected) IPv4 route; also if `tapIP` or `guestIP` equals an existing local address |
| How to observe host | `ip -j addr` and `ip -j route` (same JSON style as `DefaultIface`); parse in pure helpers for unit tests |
| Error / log text | Conflict: name the host address/route and proposed subnet; for sticky config, point at `network.tap-ip` / `network.guest-ip`. Rebase: log old → new subnet clearly. |
| CNI | Out of scope — CNI owns addressing |
| Existing fcvm TAPs | Overlap with addresses on `fcvm-tap-*` is already blocked by index + `SetupTap` existence check for the usual index↔subnet layout; rebase search must still treat those host-local/on-link entries as occupied so a rebased index 0 does not land on another live TAP's `/30` |

## Fix

1. Add host observation helpers in [network/tap.go](../../network/tap.go) (or adjacent): parse `ip -j addr` / `ip -j route` into local IPv4s and on-link prefixes; pure overlap check against a proposed `/30`. Production shells out; tests feed fixture bytes.
2. Add `network.AssertSubnetFree(tapIP, guestIP string) error` — reject overlaps (used for sticky-config path and as the predicate inside rebase).
3. Add `network.ResolveTapAddrs(baseTap, baseGuest string, index int) (tapIP, guestIP string, err error)` (or equivalent in manager):
   - Derive via `SubnetForIndex`.
   - If free → return.
   - If conflict and bases are stock defaults → try `10.200.0.1`/`10.200.0.2` at the same index, then walk third octet in `10.200`; on success return new IPs (caller logs).
   - If conflict and bases are non-default → hard error naming conflict + config keys.
   - If rebase search exhausted → hard error.
4. Call resolve/assert from `Manager.Start` on the TAP path after IPs and `hostIface` are known, before `EnableIPForward` / `SetupTap`, so a collision never mutates iptables or creates a TAP. Persist whatever IPs were chosen in state (already the case).
5. Document in [docs/network.md](../../docs/network.md) under Static TAP: default bases auto-rebase on host collision (with log); explicit bases hard-fail; nested hosts can also set `10.200.0.1` / `10.200.0.2` deliberately.

## Code touch list

| Area | Change |
|------|--------|
| [network/tap.go](../../network/tap.go) (or adjacent) | Parsers, overlap helpers, `AssertSubnetFree`, default-aware resolve/rebase |
| [vm/manager.go](../../vm/manager.go) | Resolve/assert on TAP path before host network mutation; log rebase |
| [docs/network.md](../../docs/network.md) | Note auto-rebase for defaults and hard-fail for explicit bases |

## Check to leave behind

Unit tests in `network/` with fixture JSON (no root, no live `ip`):

- Nested + defaults: host `172.16.0.2/30` (and/or on-link `172.16.0.0/30`) vs proposed default index-0 `/30` → rebase to `10.200.0.0/30` (or next free), no error.
- Nested + explicit colliding base (e.g. config `172.16.0.1` overridden to same via non-default… use a non-default colliding base like `192.168.1.1`/`192.168.1.2` against a host that owns that `/30`) → error naming the conflict.
- Disjoint: host on `172.16.0.0/30`, proposed `10.200.0.0/30` → ok (no rebase).
- Empty host addrs/routes → ok.
- Exact equality: proposed `tapIP` or `guestIP` already a local address → conflict (rebase or error per base rule).

## Non-goals

- Do not change the default `172.16.0.0/30` base for ordinary (non-colliding) hosts.
- Do not scan or skip VM indices to dodge host collisions (index still owns TAP name and jailer uid).
- Do not touch the CNI path.
- Do not try to detect "we are nested in Firecracker" as a special case; address overlap is the only signal.
- Do not auto-rebase when the operator set a non-default tap/guest base.

## Success criteria

- `fcvm start` with defaults inside a `172.16.0.2/30` guest succeeds with a non-overlapping subnet (typically `10.200.0.0/30`), does not create a colliding TAP, and the outer eth0 route still works.
- The same start with an explicit colliding `--tap-ip` / `--guest-ip` (or config) fails before creating `fcvm-tap-0` or changing iptables / `ip_forward`, with an error naming the conflict.
- Non-overlapping config proceeds as today.
- CNI-mode start is unchanged.
