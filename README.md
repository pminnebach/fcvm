# Elevator pitch

I want to build an application in Golang that can manage the lifecycle of firecracker microvm's.

# Details

This application must be able to:
- Start and stop Firecracker VM's
- Cleanup Firecracker resoures
- Download a Linux kernel from a url
- Download a rootfs from a url
- Build a custom rootfs from scratch using Docker, it should take a custom Dockerfile as parameter for different workloads
- Download the latest release of Firecracker
- Build or download the latest version of Jailer
- Take environment variables and inject them into the firecracker microvm
- Take a folder path as parameter and make it availabe inside the firecracker microvm with nfs
- Run multiple firecracker microvm's side-by-side
- Expose KVM inside the firecracker microvm with a flag
- Access any network or internet resource

After the firecracker microvm has started, i must be able to go inside the microvm and execute commands and/or applications

## Parameters

The application should be configurable with CLI flags using Cobra and Viper. The application should also be to take configuration from a config file which contains default parameters.

# Resources

Firecracker Git repository: https://github.com/firecracker-microvm/firecracker
Firecracker Getting Started: https://raw.githubusercontent.com/firecracker-microvm/firecracker/refs/heads/main/docs/getting-started.md
Firecracker Profuction Host Setup: https://raw.githubusercontent.com/firecracker-microvm/firecracker/refs/heads/main/docs/prod-host-setup.md
Firecracker Go SDK: https://github.com/firecracker-microvm/firecracker-go-sdk
Firecracker GO SDK Docs: https://pkg.go.dev/github.com/firecracker-microvm/firecracker-go-sdk

# Instructions

The LLM should follow all best practices to build this application. Refer to the skills for instructions on building the best Go applications with Viper and Cobra.
The latest Firecracker Go SDK release (1.0.0) is from 2022. Verify if you can build the SDK locally and implement it.
Do not make any assumptions, if you have any questions about a choice to be made, ask the question!
Commit after every major change.

# fcvm — Firecracker microVM manager

Built with Go, Cobra/Viper, and `firecracker-go-sdk` (latest main pseudo-version).

## Prerequisites

- Linux x86_64/aarch64 with `/dev/kvm`
- **Run as root** (jailer, tap/NAT networking, NFS exports)
- Tools: `docker`, `unsquashfs`, `mkfs.ext4`, `ssh`, `ip`, `iptables`

## Build

```bash
go build -buildvcs=false -o fcvm .
```

## Quick start

```bash
# Download firecracker + jailer
sudo ./fcvm download firecracker

# Download kernel and rootfs (use Firecracker CI URLs)
sudo ./fcvm download kernel --url 'https://.../vmlinux-...'
sudo ./fcvm download rootfs --url 'https://.../ubuntu-....squashfs'

# Start a VM (always jailed)
sudo ./fcvm start myvm --env FOO=bar --mount /data:/mnt/data

# Run commands inside the guest
sudo ./fcvm exec myvm -- uname -a
sudo ./fcvm shell myvm
sudo ./fcvm attach myvm   # serial log

# Stop and cleanup
sudo ./fcvm stop myvm
sudo ./fcvm cleanup --all
```

## Config file

Copy [fcvm.example.yaml](fcvm.example.yaml) to `~/.fcvm.yaml`. Flags and `FCVM_*` env vars override file values.

## Features

| Feature | Command / flag |
|---------|----------------|
| Start/stop VMs | `fcvm start`, `fcvm stop` |
| Always via jailer | default, no opt-out |
| Download assets | `fcvm download firecracker\|jailer\|kernel\|rootfs` |
| Docker rootfs | `fcvm build-rootfs --dockerfile path/Dockerfile` |
| Env injection | `--env KEY=VAL` or config `env:` (MMDS → guest) |
| Host folder | `--mount host:guest[:ro]` (NFS, block fallback) |
| Nested KVM | `--expose-kvm` (experimental) |
| Multi-VM | unique `--id` per VM |
| Self-check | `fcvm self-check` (skips if no KVM) |