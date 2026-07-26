# Production host checklist (operator doc)

Plan for a host-side checklist document that maps Firecracker’s [prod-host-setup.md](https://github.com/firecracker-microvm/firecracker/blob/main/docs/prod-host-setup.md) recommendations to “fcvm does X / you must do Y”, and clarifies guest SMT vs host SMT.

## Goal

Add [`docs/prod-host.md`](../docs/prod-host.md) (implement session) so operators harden the Linux host correctly. This plan is documentation-only relative to the Go binary; the deliverable of *this* backlog item is the plan + TODO link; the doc file itself is written when the TODO is executed.

## Symptom

fcvm README/docs cover CLI, network, and jailer basics but do not walk Firecracker’s host checklist (swap, KSM, host SMT, microcode, kvm-pit / `min_timer_period_us`, favordynmods, host `quiet loglevel=1`). Operators can assume `disable-smt` in fcvm config is enough — it is not.

## Root cause

Guest machine config (`disable-smt` → `MachineCfg.Smt = false` in [`vm/fc_config.go`](../vm/fc_config.go)) only affects the guest’s view of SMT. Host SMT, kernel cmdline, and sysctls are outside the fcvm process. Without an explicit mapping doc, production gaps look like “fcvm bugs” or stay invisible.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Artifact | New [`docs/prod-host.md`](../docs/prod-host.md); link from README / docs index |
| Voice | Per recommendation: **fcvm does** / **you must** / **N/A** |
| SMT | Explicit callout: guest `disable-smt` ≠ disabling SMT on the host |
| Scope | Checklist + pointers; do not auto-sysctl the host from `fcvm start` |
| Related plans | Cross-link serial/log, jailer isolation, path hardening, rate limiters, IMDS DROP, overwatcher |

## Content outline (for the future doc)

Map at least:

| Firecracker host topic | Doc treatment |
|------------------------|---------------|
| Disable swap | You must (why + how to check) |
| Disable KSM | You must |
| Disable SMT on host | You must; contrast with fcvm `disable-smt` |
| Microcode updates | You must |
| `kvm-pit` / `min_timer_period_us` | You must (sysfs/modprobe note) |
| `favordynmods` / module hardening | You must where applicable |
| Host kernel `quiet loglevel=1` | You must (host cmdline, not guest `kernel-args`) |
| Jailer / seccomp | fcvm does (always jailer; seccomp left on unless operator changes Firecracker) |
| UART / logs | fcvm partial → see [serial-log-bounding.md](serial-log-bounding.md) |
| cgroups / rlimits / per-VM uids | fcvm partial → [jailer-isolation.md](jailer-isolation.md) |
| Path permissions | fcvm partial → [jailer-path-hardening.md](jailer-path-hardening.md) |
| Rate limiters | fcvm partial → [rate-limiters.md](rate-limiters.md) |
| IMDS filter | fcvm partial → [imds-egress-filter.md](imds-egress-filter.md) |
| Overwatcher | fcvm partial → [vmm-overwatcher.md](vmm-overwatcher.md) |

## Fix (when implementing this TODO)

1. Write `docs/prod-host.md` from the outline above, citing upstream Firecracker doc.
2. Link it from [`docs/README.md`](../docs/README.md) (or docs index) and a short README “Production hosts” pointer.
3. In [`docs/configuration.md`](../docs/configuration.md) or kernel docs, one-line note under `disable-smt` pointing at the host checklist for host SMT.

## Code touch list

| Area | Change |
|------|--------|
| [`docs/prod-host.md`](../docs/prod-host.md) | New checklist (create at implement time) |
| Docs index / README | Links only |
| No Go packages | — |

## Check to leave behind

Docs-only: verify internal links resolve (README → prod-host → related plans). No Go test required.

## Non-goals

- Do not have `fcvm start` mutate host sysctls, disable swap, or rewrite host grub cmdline.
- Do not duplicate the entire upstream Firecracker doc — map and point.
- Do not implement the other production plans inside this doc task.

## Success criteria

- Operators have a single fcvm-oriented checklist for host hardening.
- Guest vs host SMT confusion is explicitly addressed.
- Each major Firecracker host recommendation is marked fcvm-does / you-must / tracked-elsewhere.
