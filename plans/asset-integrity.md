# Asset download integrity

Binaries that fcvm later executes as root are fetched with no integrity check and no timeout.

## Root cause

`DownloadFile` ([assets/download.go](../assets/download.go)) is the single fetch path for the firecracker/jailer tarball, the kernel, and rootfs images:

```go
resp, err := http.Get(url) //nolint:noctx // ponytail: simple asset fetch
```

Three problems in that one line and its surroundings:

1. **No verification.** The tarball is unpacked to `~/.fcvm/bin/firecracker` and `jailer` with mode `0755`, and every subsequent `fcvm start` execs them as root. A compromised mirror, a hijacked redirect, or a corrupted transfer is undetectable.
2. **No timeout.** `http.Get` and the `http.DefaultClient` call in `LatestFirecrackerRelease` have no client timeout, so a stalled server hangs `fcvm download` forever with no way to interrupt it (the commands do not use `cmd.Context()` either — see [cli-ergonomics.md](cli-ergonomics.md)).
3. **No resume or progress.** Kernel and rootfs images are hundreds of MB; a failed transfer restarts from zero with no feedback. Lower priority, but the same function.

`DownloadKernel` is a bare pass-through to `DownloadFile`, and the kernel URL is user-supplied (`--url` / `kernel-url`), so a plain `http://` URL is accepted silently.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Firecracker release | Verify against the checksum file published alongside the release artifact; fail closed |
| Kernel / rootfs | Optional `--sha256` flag and `kernel-sha256` config; verified when set |
| Unverifiable downloads | Allowed only when no checksum is available *and* the URL is HTTPS; print a clear warning naming the file |
| Plain HTTP | Rejected unless `--insecure` is passed |
| Client | One shared `*http.Client` with a timeout, used by every fetch in the package |
| Context | `http.NewRequestWithContext`; `ctx` threaded from the command |
| Hash-while-writing | Stream through `io.MultiWriter(file, hasher)`; no second pass over the file |
| Failed verification | Delete the `.tmp` file, return an error naming expected vs actual |

## Fix

1. Replace the package-level `http.Get`/`http.DefaultClient` uses with a single client that has a timeout, and give `DownloadFile` a `context.Context` and an optional expected digest.
2. Hash while copying — `DownloadFile` already writes to `dest + ".tmp"` and renames on success, so verification slots in cleanly before `os.Rename`. That ordering also means a failed check never leaves a usable binary behind.
3. For `DownloadFirecracker`, fetch the release's published checksum file for the resolved tag before downloading the tarball, and verify. If the checksum file is absent for that tag, fail with an explanatory error rather than proceeding silently.
4. Verify the extracted `firecracker` and `jailer` binaries too if the release publishes per-binary digests; otherwise verifying the tarball is sufficient since extraction is local.
5. Add the URL scheme check in one place so `download kernel` and `download rootfs` both get it.
6. `LatestFirecrackerRelease` follows a redirect to determine the tag; give it the same client and context, and keep its existing "could not parse release tag" error.

## Code touch list

| Area | Change |
|------|--------|
| [assets/download.go](../assets/download.go) | Shared client with timeout; ctx + digest parameters; hash-while-writing; scheme check; checksum fetch for releases |
| [assets/kernel_ci.go](../assets/kernel_ci.go) | Same client/context for the S3 listing calls |
| [cmd/download.go](../cmd/download.go) | `--sha256` and `--insecure` flags; pass `cmd.Context()` |
| [config/config.go](../config/config.go) | `kernel-sha256` (and rootfs equivalent if wanted) |
| [docs/install.md](../docs/install.md) / [docs/kernel.md](../docs/kernel.md) | Document verification behaviour and how to supply a digest |

## Check to leave behind

One test in `assets/` using `httptest.NewServer` — no network, fast:

- serve known bytes, download with the correct digest, assert success and that the file exists;
- download the same bytes with a wrong digest, assert an error **and** that no file was left at the destination.

The second assertion is the one that matters: verification that leaves the bad artifact in place is not verification.

## Non-goals

- No GPG/sigstore signature verification (checksums over HTTPS are the proportionate step).
- No mirror or proxy configuration.
- No resumable downloads or progress bars in this pass.
- Do not vendor or pin a specific Firecracker version; `latest` resolution stays.

## Success criteria

- A tampered or truncated firecracker tarball fails the download with a clear message and leaves nothing executable behind.
- `fcvm download kernel --url http://…` is refused without `--insecure`.
- A hung server aborts on the client timeout instead of blocking forever, and Ctrl+C works.
