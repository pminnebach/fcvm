# External sources and licenses

fcvm itself is MIT — see [LICENSE](../LICENSE).

## Direct Go modules

From the top-level `require` block in [`go.mod`](../go.mod):

| Module | Role | License |
|--------|------|---------|
| [github.com/firecracker-microvm/firecracker-go-sdk](https://github.com/firecracker-microvm/firecracker-go-sdk) | Firecracker machine API | Apache-2.0 |
| [github.com/containernetworking/cni](https://github.com/containernetworking/cni) | CNI network mode | Apache-2.0 |
| [github.com/sirupsen/logrus](https://github.com/sirupsen/logrus) | Logging | MIT |
| [github.com/spf13/cobra](https://github.com/spf13/cobra) | CLI | Apache-2.0 |
| [github.com/spf13/viper](https://github.com/spf13/viper) | Config / flags / env | Apache-2.0 |
| [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) | SSH guest access | BSD-3-Clause |
| [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys) | OS primitives | BSD-3-Clause |

Transitive modules follow their own licenses (see each module’s repository or `go.mod` graph).

## Runtime downloads

Fetched by `install.sh` / `fcvm download`; not compiled into the fcvm binary:

| Asset | Source | License / notes |
|-------|--------|-----------------|
| Firecracker + jailer | [firecracker-microvm/firecracker](https://github.com/firecracker-microvm/firecracker) GitHub releases | Apache-2.0 |
| Stock guest kernel (`vmlinux`) | Firecracker CI artifacts on `s3.amazonaws.com/spec.ccfc.min` | Published by the Firecracker project for CI; see [kernel.md](kernel.md) |
