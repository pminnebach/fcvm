# TODO

[x] Add parameter to change rootfs size at build.
[ ] Rewrite readme with clear run instructions.
[ ] Rewrite BUILD.md with better build and install instruction.
[ ] Add ARCHITECTURE.md with the architecture of the application, config file explenation and ~/.fcvm folder explenation.
[x] Add version command in the application.
[ ] Fix mounted folder being emptied upon microvm crash or running "fcvm clenanup [--all]"
[ ] Sync block-fallback mount images back to the host directory on stop
[ ] Expose unused Firecracker jailer isolation knobs (see plans/jailer-isolation.md)
[ ] Implement optional CNI networking (see plans/cni-network.md)
[ ] Expose-kvm doesn't do anything. KVM always works in a microvm with a kvm enabled kernel.