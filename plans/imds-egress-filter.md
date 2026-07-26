# Host IMDS egress DROP

Block guest packets to the link-local AWS (and compatible) instance metadata address `169.254.169.254` on the TAP/iptables path. Firecracker MMDS inside the guest keeps working; only host IMDS is denied.

## Goal

Add a DROP rule on the `FCVM` chain for guest → `169.254.169.254` **before** ACCEPT rules, with symmetric teardown. Document the equivalent expectation for CNI users.

## Symptom

On cloud hosts, a guest that can reach the host’s IMDS endpoint can steal the host instance role credentials. Firecracker prod-host-setup recommends filtering this; fcvm’s TAP setup only adds ACCEPT + MASQUERADE for the guest subnet ([`network/tap.go`](../network/tap.go)).

## Root cause

`vmRules` installs forward ACCEPT rules for the VM subnet and never inserts a more-specific DROP for the metadata IP. Guests use the host as a router; without an explicit DROP, IMDS is reachable like any other link-local hop the host answers.

MMDS is different: Firecracker intercepts MMDS inside the VMM for the guest NIC (`AllowMMDS: true` in [`vm/fc_config.go`](../vm/fc_config.go)). Blocking host IMDS on the TAP bridge does not remove MMDS.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Target | `169.254.169.254` (IPv4 IMDS). IPv6 IMDS out of scope unless already trivial |
| Placement | `FCVM` chain, **before** per-VM ACCEPT rules (insert at head or order rules so DROP matches first) |
| Scope | Per-VM or shared DROP both OK if teardown is correct; prefer a single shared DROP in `FCVM` with refcount, or one DROP per guest subnet — pick the smaller correct option at implement time |
| Teardown | Delete the DROP rule symmetrically when the VM (or last VM) stops — same mirror discipline as [host-network-scope.md](host-network-scope.md) |
| CNI | Do not silently inject iptables into CNI nets; document that the CNI network must DROP host IMDS (or provide a sample CNI conflist snippet in docs) |
| MMDS | Keep `AllowMMDS: true`; docs clarify host IMDS vs guest MMDS |

## Fix

1. Extend `vmRules` / `ensureFCVMChain` helpers in [`network/tap.go`](../network/tap.go) to add DROP for traffic to `169.254.169.254` (appropriate iface/subnet match so only guest egress is affected).
2. Ensure rule order: DROP before ACCEPT.
3. Teardown deletes the same rule args with `-D`.
4. Extend argv-builder unit tests so add/delete forms stay mirrors (pattern already used in `network/tap_test.go`).
5. Docs: [`docs/network.md`](../docs/network.md) — host IMDS blocked; MMDS still available; CNI note.

## Code touch list

| Area | Change |
|------|--------|
| [`network/tap.go`](../network/tap.go) | DROP rule construction + ordering + teardown |
| [`network/tap_test.go`](../network/tap_test.go) | Mirror assert for DROP argv |
| [`docs/network.md`](../docs/network.md) | IMDS vs MMDS + CNI |

## Check to leave behind

Assert the DROP rule argv (add and delete) in the existing pure unit tests. Optionally assert it appears before ACCEPT in the ordered `vmRules` slice.

## Non-goals

- No general egress firewall / allowlist.
- No IPv6 IMDS unless one extra rule is trivial and tested.
- No change to MMDS configuration or guest metadata contents.
- No host-wide OUTPUT chain rules for non-fcvm traffic.

## Success criteria

- Guest cannot reach host `169.254.169.254` via the TAP path while the VM is up.
- Guest MMDS (Firecracker) still works.
- After stop/teardown, the DROP rule is gone (no orphans).
- CNI operators have a documented expectation.
