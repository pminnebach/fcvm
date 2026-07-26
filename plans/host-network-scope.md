# Host network scope

TAP setup makes global, permanent changes to the host's packet policy and never undoes them. Scope the rules to fcvm's own traffic and remove them on teardown.

## Symptom

Starting one VM changes the host for every other workload on it, and stopping the VM does not change it back:

```bash
sudo iptables -S FORWARD | head -1     # -P FORWARD DROP   (e.g. a Docker host)
sudo fcvm start a
sudo iptables -S FORWARD | head -1     # -P FORWARD ACCEPT
sudo fcvm stop a
sudo iptables -S FORWARD | head -1     # still ACCEPT
sudo iptables -t nat -S POSTROUTING    # MASQUERADE rule still present
```

`/proc/sys/net/ipv4/ip_forward` is likewise flipped to 1 and never restored.

## Root cause

`SetupTap` ([network/tap.go](../network/tap.go)) reaches for the broadest possible switches:

```go
if err := run("sh", "-c", "echo 1 > /proc/sys/net/ipv4/ip_forward"); err != nil { … }
if err := run("iptables", "-P", "FORWARD", "ACCEPT"); err != nil { … }
…
run("iptables", "-t", "nat", "-A", "POSTROUTING", "-o", hostIface, "-j", "MASQUERADE")
```

Setting the chain *policy* to ACCEPT is a host-wide security downgrade: it affects every forwarded packet, not just the guest's, and on a host where something else deliberately set DROP it silently removes that protection. `TeardownTap` only runs `ip link del`, so all three changes outlive the VM.

Two supporting problems:

- The MASQUERADE rule is deliberately shared across VMs (delete-then-add on every start), so no single VM's teardown can safely remove it. There is no reference counting.
- `defaultIface` string-scans the output of `ip -j route list default` for `"dev":` and falls back to a hardcoded `eth0` when parsing fails. It asks for JSON and then does not parse it; a wrong answer here points the MASQUERADE rule at the wrong interface. The `ponytail:` comment already names `json.Unmarshal` as the upgrade path, and it is shorter than the current code.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Chain policy | Never touch `-P FORWARD`; leave the host default alone |
| Rules | Explicit ACCEPT rules for the VM's `/30` in a dedicated `FCVM` chain, jumped from `FORWARD` |
| Chain lifecycle | Create the `FCVM` chain and its `FORWARD` jump on first use; per-VM rules added on start, deleted on stop |
| Chain removal | Remove the jump and flush the chain when the last VM goes away (`cleanup --all` and the last `stop`) |
| ip_forward | Record the prior value in state on first enable; restore when the last VM stops |
| MASQUERADE | One rule per VM subnet rather than one global rule, so teardown is per-VM and exact |
| Interface detection | Parse `ip -j route` JSON; error out rather than guessing `eth0` |
| nftables hosts | Out of scope; keep using the `iptables` binary (nft-compat covers most hosts) |

## Fix

1. Add `ensureFCVMChain()` that is idempotent: create `FCVM` if absent, ensure exactly one `-A FORWARD -j FCVM` jump.
2. Replace the policy change with two rules per VM in the `FCVM` chain — guest subnet out to the host interface, and the established/related return path.
3. Replace the shared MASQUERADE with `-s <guest-subnet> -o <hostIface> -j MASQUERADE`, added on start and deleted by exact match on teardown.
4. Give `TeardownTap` the VM's subnet and host interface so it can delete precisely what `SetupTap` added. Both are derivable from state (`GuestIP`, plus the recorded index from [vm-index-allocation.md](vm-index-allocation.md)); persist the host interface in `state.json` so teardown does not have to re-detect it after the default route has changed.
5. Save the pre-existing `ip_forward` value the first time fcvm enables it (a small marker file under `<state-dir>`), and restore it when no VMs remain.
6. Rewrite `defaultIface` around `json.Unmarshal` into a `[]struct{ Dev string \`json:"dev"\` }` and return an error when the list is empty instead of returning `eth0`.

## Code touch list

| Area | Change |
|------|--------|
| [network/tap.go](../network/tap.go) | `FCVM` chain helpers; per-VM accept + masquerade rules; precise teardown; JSON `defaultIface`; drop the `-P FORWARD` call and the empty `if` body above it |
| [vm/state.go](../vm/state.go) | Persist host interface (and subnet) used for the VM's rules |
| [vm/manager.go](../vm/manager.go) | Pass teardown parameters; restore `ip_forward` when the last VM stops |
| [docs/network.md](../docs/network.md) | Document the `FCVM` chain, what is added per VM, and what is restored |

## Check to leave behind

The rule *construction* is the part that can silently regress, and it is pure string building. Extract the argv builders (accept rule, masquerade rule) and assert in a `network/` test that the add and delete forms are exact mirrors — a teardown that does not match its setup is how orphan rules accumulate. That runs without root.

Also assert `defaultIface`'s parser against a captured `ip -j route list default` JSON fixture string, including the empty-list case.

## Non-goals

- No nftables backend, no firewalld integration.
- Do not attempt to detect or coexist with other tools' rules beyond using our own chain.
- Do not make the guest reachable from outside the host (no port forwarding here).
- Do not change the CNI path — plugins own their own rules.

## Success criteria

- `iptables -S FORWARD | head -1` is unchanged by a start/stop cycle.
- After `fcvm stop`, no fcvm rule remains in `FCVM` or `nat/POSTROUTING` for that VM.
- After the last VM stops, `ip_forward` is back to its original value and the `FCVM` chain is empty.
- Guest egress still works while the VM is running.
