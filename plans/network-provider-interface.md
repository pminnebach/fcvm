# NetworkProvider interface (future release, not started)

Formalize the `useCNI` boolean that's threaded through `vm/manager.go`, `vm/fc_config.go`, `vm/state.go` and `rootfs/hooks.go` into a proper `NetworkProvider` interface, with `tapProvider` and `cniProvider` as its two implementations. This is explicitly **not** part of the CNI hardening work tracked in `TODO.md` / [cni-network.md](done/cni-network.md) — it was deferred out of that pass on the reasoning that a formal interface is much easier to get right once CNI's own behavior is fully nailed down, tested, and has seen real use from behind `--enable-experimental`. Treat this doc as a starting point for whoever picks it up next, not a committed design.

## Why

Today, "which network mode" is a plain `if useCNI { ... } else { ... }` repeated at every point where the two modes diverge. That's fine at two modes and a handful of branch points, but it has two costs:

- **Adding a third mode later** (a plain bridge, a user-supplied hook, whatever) means finding and updating every one of those branch points, in every file, without missing one.
- **The two modes drift in how carefully they're handled.** TAP has a decade of hardening (subnet collision checks, host-scoped firewall rules, index allocation) that's easy to forget to carry over to a new branch in a `useCNI` conditional, because there's no shared shape forcing parity.

A `NetworkProvider` interface would make that parity structural: both `tapProvider` and `cniProvider` implement the same methods, so a change to one interface method's contract is visible (a compile error, a missed test) if the other implementation doesn't handle it.

## Branch points to replace

From reading the current implementation (`useCNI := m.cfg.Network.CNINetwork != ""` in `vm/manager.go:102`):

| File | What branches on `useCNI` today |
|------|-------------------------------|
| `vm/manager.go` (~102, 110, 160, 172, 180, 253, 297, 581) | Index/address allocation (TAP derives a `/30`, CNI has none), rootfs network patch skip, `TeardownNFSExportsForVM`/`SetupTap` vs. CNI preflight, `failCleanup`'s teardown choice, post-`Start` address resolution (`resolveCNIAddrs` vs. static values already known), `state.NetworkMode` assignment, `teardownState`'s `TeardownCNI` vs. `TeardownTap` choice |
| `vm/fc_config.go:120` | `buildNetworkInterfaces` — returns a `CNIConfiguration` or a `StaticConfiguration` `firecracker.NetworkInterface` |
| `vm/state.go` | `NetworkMode` string constant (`"tap"`/`"cni"`), `IsCNI()` |
| `rootfs/hooks.go` | `PatchOptions.StaticNetwork` bool — skips `/etc/fcvm/network` injection for CNI |

## Sketch

```go
type NetworkProvider interface {
    // Setup does whatever host-side work this mode needs before the machine
    // is built (TAP: EnableIPForward + SetupTap; CNI: ValidateCNIPrereqs).
    Setup(ctx context.Context, in setupInput) (setupResult, error)

    // BuildInterface returns the firecracker.NetworkInterface for this mode.
    BuildInterface(cfg config.Config, in machineBuildInput) firecracker.NetworkInterface

    // ResolveAddrs fills in guest IP/gateway/MAC after Start, when they
    // weren't already known before boot (a no-op for TAP, real work for CNI).
    ResolveAddrs(machine *firecracker.Machine, in setupResult) (guestIP, gateway, mac string, err error)

    // Teardown reverses Setup; called from Stop/Cleanup.
    Teardown(ctx context.Context, state *State)

    // NetworkMode is the value persisted to state.json.
    NetworkMode() string
}
```

The exact method shapes above are a starting sketch, not a decision — `setupInput`/`setupResult` in particular need to be designed against what `vm/manager.go`'s `Start` actually needs at each stage (index, uid/gid, and rootfs patch options are currently interleaved with TAP-specific address derivation in ways that would need untangling). Whoever picks this up should re-derive the exact interface from the current code rather than treating this sketch as final.

## Non-goals (for this future work, when it happens)

- Don't let this become an excuse to also change TAP's or CNI's actual behavior — it's a structural refactor, not a rewrite. Land it as a behavior-preserving change verified against the existing test suite plus the CNI hardening pass's new tests.
- Don't design for a third mode that doesn't exist yet beyond what "two clean implementations of one interface" naturally gives you for free.

## Prerequisite

The CNI hardening work tracked in `TODO.md` (test coverage parity with TAP for `network/cni.go` and `resolveCNIAddrs`, the preflight check, the NFS-gateway fix, the docs rewrite) should land first. Extracting an interface from code that's already well-tested and whose edge cases are already understood is a much smaller, safer change than extracting one from code that's still being hardened.
