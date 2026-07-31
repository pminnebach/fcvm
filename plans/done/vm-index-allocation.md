# VM index allocation

Stop deriving the VM network index from the *count* of running VMs. Index reuse makes a new VM delete a running VM's TAP device and claim its guest IP.

## Symptom

```bash
sudo fcvm start a      # index 0 → fcvm-tap-0, 172.16.0.2
sudo fcvm start b      # index 1 → fcvm-tap-1, 172.16.1.2
sudo fcvm stop a       # one state dir left
sudo fcvm start c      # index 1 again → deletes b's TAP, reuses b's IP
```

VM `b` loses its network and never recovers. With `jailer.per-vm-uids: true`, `b` and `c` also share a jailer uid/gid, silently removing the isolation that flag exists to provide.

## Root cause

`Manager.nextVMIndex` ([vm/manager.go](../../vm/manager.go)) returns `len(states)`, which is a population count, not an allocation:

```go
func (m *Manager) nextVMIndex() int {
	states, _ := ListStates(m.cfg.StateDir)
	return len(states)
}
```

The index feeds three things that must be unique across live VMs — `network.SubnetForIndex`, `network.TapDevName`, and `jailerCreds` — and `SetupTap` ([network/tap.go](../../network/tap.go)) opens by deleting whatever device already holds the name:

```go
if err := run("ip", "link", "del", tapDev); err != nil && !strings.Contains(err.Error(), "Cannot find device") {
	// ignore missing tap
}
```

So a colliding index is not a start failure, it is a silent takeover. Two secondary defects sit on the same code path:

- The allocated index is never persisted, so nothing can reconstruct or audit it after the fact — `state.json` records `TapDev` but not the index that produced the IP and uid.
- `SubnetForIndex` wraps silently: `tap[2] = byte(int(tap[2]) + index)` overflows past index 239 on the default `172.16.0.1` base and collides again with no error.

There is also no locking around `Start`, so two concurrent `fcvm start` invocations read the same state directory and race to the same index even with correct allocation logic.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Allocation | Lowest non-negative index not claimed by an existing `state.json` |
| Source of truth | New `Index int` field in `vm.State`; scan `ListStates` for claimed indices |
| Legacy states | State without `Index` (older VMs) → derive from `TapDev` suffix, else treat as claiming nothing |
| Collision at setup | `SetupTap` must fail if the device already exists rather than delete it |
| Overflow | `SubnetForIndex` returns an error above the usable range instead of wrapping |
| Concurrency | Single lock file under `StateDir` held across allocate+`SaveState` |
| CNI mode | Index still allocated for jailer uid/gid; TAP/IP derivation stays skipped |

## Fix

1. Add `Index int \`json:"index"\`` to `vm.State` ([vm/state.go](../../vm/state.go)).
2. Replace `nextVMIndex` with an allocator that builds a set of claimed indices from `ListStates` and returns the lowest free one. Keep it a method on `Manager` so the state dir stays injected.
3. Persist `Index` in the `State` literal in `Start`.
4. Change `SubnetForIndex` to `(tapIP, guestIP string, err error)` and reject an index that would push the third octet past 255. `Start` surfaces the error instead of booting a colliding VM.
5. In `SetupTap`, replace the leading `ip link del` with an existence check that returns an error naming the conflicting device. Orphan reclaim stays the job of `fcvm cleanup`, which already knows the TAP name from state.
6. Serialize allocation: take an exclusive `flock` on `<state-dir>/.lock` at the top of `Start`, release after `SaveState`. This is the smallest thing that closes the concurrent-start race; the whole start path is already effectively serial per host.

## Code touch list

| Area | Change |
|------|--------|
| [vm/state.go](../../vm/state.go) | `Index` field; helper returning claimed indices from `ListStates` |
| [vm/manager.go](../../vm/manager.go) | `nextVMIndex` → lowest-free allocator; persist `Index`; handle `SubnetForIndex` error; take the state lock |
| [network/tap.go](../../network/tap.go) | `SubnetForIndex` returns an error on overflow; `SetupTap` refuses an existing device |
| [docs/network.md](../../docs/network.md) | Correct the "index" description: allocation is lowest-free, not sequential by count |
| [docs/architecture.md](../../docs/architecture.md) | `state.json` field list gains `index` |

## Check to leave behind

One test in `vm/`: save states claiming indices 0 and 2, assert the allocator returns 1, then save a state at 1 and assert it returns 3. That is the exact case `len(states)` gets wrong, and it needs no root, no network, and no Firecracker.

A second cheap assertion in `network/`: `SubnetForIndex` with an index that would overflow the third octet returns an error rather than a wrapped address.

## Non-goals

- No index recycling policy beyond lowest-free (no LRU, no reservation TTL).
- Do not change the `/30`-per-VM addressing scheme or the third-octet layout.
- Do not add a daemon or shared allocator service; the lock file is enough.
- Do not touch CNI address assignment — the SDK owns that.

## Success criteria

- The start/stop/start sequence above leaves VM `b` with a working TAP and its original IP.
- Starting a VM whose index would collide with a live device fails with a named error instead of taking the device over.
- `state.json` records the index used, and `cleanup --all` still reclaims everything.
