# TODO

## Open

- [ ] Interactive serial console via screen (see [plans/serial-console.md](plans/serial-console.md))
- [ ] Expose unused Firecracker jailer isolation knobs, including `--resource-limit` / `--new-pid-ns` via custom argv, a recommended stock cgroup recipe in examples/docs (not silently forced), and production guidance for `per-vm-uids: true` while keeping the compatibility default false (see [plans/jailer-isolation.md](plans/jailer-isolation.md))
- [ ] Implement optional CNI networking (see [plans/cni-network.md](plans/cni-network.md))

### Production host conformance

Firecracker [prod-host-setup](https://github.com/firecracker-microvm/firecracker/blob/main/docs/prod-host-setup.md) gaps tracked as plans. Suggested order: IMDS DROP and path hardening first; rate limiters + serial/log next; resource-limits/cgroup recipe with jailer-isolation (above); overwatcher; host checklist anytime.

- [ ] Bound serial UART + Firecracker logs for production (see [plans/serial-log-bounding.md](plans/serial-log-bounding.md); conflicts with interactive console when UART is off)
- [ ] Validate jailer/firecracker/chroot paths are not group/world-writable (see [plans/jailer-path-hardening.md](plans/jailer-path-hardening.md))
- [ ] Wire NIC/drive rate limiters via config (see [plans/rate-limiters.md](plans/rate-limiters.md))
- [ ] DROP guest egress to host IMDS (`169.254.169.254`) on the FCVM chain (see [plans/imds-egress-filter.md](plans/imds-egress-filter.md))
- [ ] Stuck-VMM overwatcher subcommand (see [plans/vmm-overwatcher.md](plans/vmm-overwatcher.md))
- [ ] Operator host checklist doc (`docs/prod-host.md`) — guest `disable-smt` ≠ host SMT (see [plans/prod-host-checklist.md](plans/prod-host-checklist.md))

### Follow-ups from the review

- [ ] Move `cmd/` off the global viper singleton so config does not leak between tests (noted in [plans/cli-ergonomics.md](plans/cli-ergonomics.md); the `cmd.OutOrStdout()` groundwork is done)
- [ ] Consider an in-process SSH client via `x/crypto/ssh` instead of shelling out to `ssh` (argument quoting is fixed either way)
- [ ] `copyFile` is not sparse-aware, so every `start` writes the full rootfs size (see [plans/repo-hygiene.md](plans/repo-hygiene.md))
- [ ] Coverage is still thin in `assets/` and `cmd/`

## Done

- [x] Add parameter to change rootfs size at build.
- [x] Rewrite docs into docs/ (architecture, network, kernel, rootfs, install, cli, configuration) and slim README.
- [x] Add architecture / config / ~/.fcvm layout docs (see docs/).
- [x] Add version command in the application.
- [x] Fix mounted folder being emptied upon microvm crash or running "fcvm cleanup [--all]"
- [x] Expose-kvm doesn't do anything. KVM always works in a microvm with a kvm enabled kernel.
- [x] Fix VM index allocation — index reuse let a new VM delete a running VM's TAP and steal its IP ([plans/vm-index-allocation.md](plans/vm-index-allocation.md))
- [x] Stop losing guest writes on block-fallback mounts; the fallback is now opt-in and syncs back on stop ([plans/mount-writeback.md](plans/mount-writeback.md))
- [x] Validate VM ids before they reach `os.RemoveAll` as root, and guard the remaining mount/`RemoveAll` pairs ([plans/destructive-path-guards.md](plans/destructive-path-guards.md))
- [x] Make `stop` PID-safe and give `list` a real status column ([plans/vm-liveness.md](plans/vm-liveness.md))
- [x] Scope NFS exports to the guest instead of `*`, and move staging out of `/tmp` ([plans/nfs-export-hardening.md](plans/nfs-export-hardening.md))
- [x] Stop flipping the host's `FORWARD` policy and `ip_forward`; use a dedicated chain and clean up on teardown ([plans/host-network-scope.md](plans/host-network-scope.md))
- [x] Verify downloaded assets before executing them as root; add HTTP timeouts ([plans/asset-integrity.md](plans/asset-integrity.md))
- [x] CLI ergonomics: context/signal handling, `SilenceUsage`, strict mount parsing, `exec` argument quoting ([plans/cli-ergonomics.md](plans/cli-ergonomics.md))
- [x] Guest bootstrap: configurable DNS, no shell JSON parsing, quote-safe env injection ([plans/guest-bootstrap.md](plans/guest-bootstrap.md))
- [x] Add CI (gofmt, vet, test, build) ([plans/repo-hygiene.md](plans/repo-hygiene.md))
- [x] Delete dead code, fix always-nil error and the `docker create` panic, repoint the fake network-config test, stop root tests from mutating the host ([plans/repo-hygiene.md](plans/repo-hygiene.md))
