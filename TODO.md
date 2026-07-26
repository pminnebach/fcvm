# TODO

- [x] Add parameter to change rootfs size at build.
- [x] Rewrite docs into docs/ (architecture, network, kernel, rootfs, install, cli, configuration) and slim README.
- [x] Add architecture / config / ~/.fcvm layout docs (see docs/).
- [x] Add version command in the application.
- [x] Fix mounted folder being emptied upon microvm crash or running "fcvm clenanup [--all]"
- [ ] Sync block-fallback mount images back to the host directory on stop
- [ ] Expose unused Firecracker jailer isolation knobs (see plans/jailer-isolation.md)
- [ ] Implement optional CNI networking (see plans/cni-network.md)
- [x] Expose-kvm doesn't do anything. KVM always works in a microvm with a kvm enabled kernel.
