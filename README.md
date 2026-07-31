# fcvm

CLI for managing Firecracker microVM lifecycle on Linux: download Firecracker/jailer/kernel/rootfs, build custom rootfs images from Dockerfiles, start and stop jailed VMs, inject env and host mounts, and run commands in guests over SSH.

**Requires root and `/dev/kvm`.** Every VM runs through the Firecracker jailer.

## Quick start

```bash
curl -sSL https://raw.githubusercontent.com/pminnebach/fcvm/refs/heads/main/install.sh | sudo bash
sudo fcvm build-rootfs --dockerfile ./Dockerfile
sudo fcvm start myvm
sudo fcvm exec myvm -- uname -a
sudo fcvm stop myvm
```

Or build from source:

```bash
go build -buildvcs=false -o fcvm .
sudo ./fcvm download firecracker
sudo ./fcvm download kernel
sudo ./fcvm build-rootfs --dockerfile ./Dockerfile
sudo ./fcvm start myvm
```

See [docs/install.md](docs/install.md) for host dependencies, GoReleaser builds, and asset options.

## Documentation

| Doc | Topic |
|-----|--------|
| [docs/architecture.md](docs/architecture.md) | Packages, lifecycle, jailer, state layout |
| [docs/network.md](docs/network.md) | TAP+NAT, CNI, NFS mounts |
| [docs/kernel.md](docs/kernel.md) | Stock CI kernel and custom KVM builds |
| [docs/rootfs.md](docs/rootfs.md) | Docker → ext4, downloads, guest hooks |
| [docs/install.md](docs/install.md) | Build, install, first-time setup |
| [docs/cli.md](docs/cli.md) | Command and flag reference |
| [docs/configuration.md](docs/configuration.md) | Config file, env, defaults |
| [docs/debug.md](docs/debug.md) | Inspect VMs from the host without the CLI |

Example config: [fcvm.example.yaml](fcvm.example.yaml).

## License

See [LICENSE](LICENSE).
