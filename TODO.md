# TODO

## Open

- [ ] Interactive serial console via screen (see [plans/serial-console.md](plans/serial-console.md))
- [ ] Expose unused Firecracker jailer isolation knobs (see [plans/jailer-isolation.md](plans/jailer-isolation.md))
- [ ] Implement optional CNI networking (see [plans/cni-network.md](plans/cni-network.md))
- [ ] Refuse TAP start when the guest /30 overlaps host addresses/routes (nested sandbox stay-alive; see [plans/host-subnet-collision.md](plans/host-subnet-collision.md))

### Follow-ups from the review

- [ ] Move `cmd/` off the global viper singleton so config does not leak between tests (noted in [plans/done/cli-ergonomics.md](plans/done/cli-ergonomics.md); the `cmd.OutOrStdout()` groundwork is done)
- [ ] Consider an in-process SSH client via `x/crypto/ssh` instead of shelling out to `ssh` (argument quoting is fixed either way)
- [ ] `copyFile` is not sparse-aware, so every `start` writes the full rootfs size (see [plans/done/repo-hygiene.md](plans/done/repo-hygiene.md))
- [ ] Coverage is still thin in `assets/` and `cmd/`

## Done

- [x] Add parameter to change rootfs size at build.
- [x] Rewrite docs into docs/ (architecture, network, kernel, rootfs, install, cli, configuration) and slim README.
- [x] Add architecture / config / ~/.fcvm layout docs (see docs/).
- [x] Add version command in the application.
- [x] Fix mounted folder being emptied upon microvm crash or running "fcvm cleanup [--all]"
- [x] Expose-kvm doesn't do anything. KVM always works in a microvm with a kvm enabled kernel.
- [x] Fix VM index allocation — index reuse let a new VM delete a running VM's TAP and steal its IP ([plans/done/vm-index-allocation.md](plans/done/vm-index-allocation.md))
- [x] Stop losing guest writes on block-fallback mounts; the fallback is now opt-in and syncs back on stop ([plans/done/mount-writeback.md](plans/done/mount-writeback.md))
- [x] Validate VM ids before they reach `os.RemoveAll` as root, and guard the remaining mount/`RemoveAll` pairs ([plans/done/destructive-path-guards.md](plans/done/destructive-path-guards.md))
- [x] Make `stop` PID-safe and give `list` a real status column ([plans/done/vm-liveness.md](plans/done/vm-liveness.md))
- [x] Scope NFS exports to the guest instead of `*`, and move staging out of `/tmp` ([plans/done/nfs-export-hardening.md](plans/done/nfs-export-hardening.md))
- [x] Stop flipping the host's `FORWARD` policy and `ip_forward`; use a dedicated chain and clean up on teardown ([plans/done/host-network-scope.md](plans/done/host-network-scope.md))
- [x] Verify downloaded assets before executing them as root; add HTTP timeouts ([plans/done/asset-integrity.md](plans/done/asset-integrity.md))
- [x] CLI ergonomics: context/signal handling, `SilenceUsage`, strict mount parsing, `exec` argument quoting ([plans/done/cli-ergonomics.md](plans/done/cli-ergonomics.md))
- [x] Guest bootstrap: configurable DNS, no shell JSON parsing, quote-safe env injection ([plans/done/guest-bootstrap.md](plans/done/guest-bootstrap.md))
- [x] Add CI (gofmt, vet, test, build) ([plans/done/repo-hygiene.md](plans/done/repo-hygiene.md))
- [x] Delete dead code, fix always-nil error and the `docker create` panic, repoint the fake network-config test, stop root tests from mutating the host ([plans/done/repo-hygiene.md](plans/done/repo-hygiene.md))
