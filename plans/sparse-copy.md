# Sparse-aware rootfs copy

## Problem

`copyFile` in `vm/manager.go` (around lines 622-637) does a plain `out.ReadFrom(in)` full-content copy. Rootfs images built by `build-rootfs`/`download` are typically sparse ext4 images (mostly zero-filled unused blocks); a naive copy materializes every hole as real zero bytes on disk, so every `fcvm start` writes the image's full nominal size to disk, multiplying disk usage per running VM even though the source images are mostly empty space.

## Goal

Make the per-`start` rootfs copy sparse-aware so holes in the source stay holes in the destination, reducing both copy time and on-disk footprint, without changing correctness (the copied file must read back identically to a full copy).

## Design options (pick one at implement time based on what's simplest/most portable)

1. **`FICLONE` reflink (Linux-specific, fastest, needs same filesystem):** attempt an ioctl-based reflink clone first for instant CoW copies when source and destination are on the same filesystem (common case: both under `~/.fcvm/`); fall back to (2) if it fails (different filesystems, unsupported fs type).
2. **`SEEK_HOLE`/`SEEK_DATA` hole-skipping copy (portable baseline):** walk the source file's data/hole segments via `unix.Seek` with `unix.SEEK_DATA`/`unix.SEEK_HOLE` (from `golang.org/x/sys/unix` — Go's stdlib `io` package does not expose these), `Seek`ing the destination forward over hole regions (creating real sparseness on most filesystems as long as zero bytes are never written into those ranges) and only copying the actual data segments.
3. **`copy_file_range(2)` syscall (avoids userspace copy, but does not preserve sparseness by itself — combine with (2)'s hole detection to skip ranges).**

Recommended default: implement (2) as the portable baseline first; only add (1) as a fast-path optimization if `start` latency on real hardware shows the hole-skip copy alone isn't fast enough. Don't over-engineer this on the first pass.

## Implementation sketch

```go
func copyFile(src, dst string) error {
    in, err := os.Open(src)
    if err != nil { return err }
    defer in.Close()

    out, err := os.Create(dst)
    if err != nil { return err }
    defer out.Close()

    size, err := in.Seek(0, io.SeekEnd)
    if err != nil { return err }
    if _, err := in.Seek(0, io.SeekStart); err != nil { return err }

    var pos int64
    for pos < size {
        // dataStart/holeStart via unix.Seek(fd, pos, unix.SEEK_DATA) / unix.SEEK_HOLE,
        // not the stdlib io.Seek* constants (those don't include SEEK_DATA/SEEK_HOLE).
        dataStart, err := seekData(in, pos)
        if err != nil {
            break // ENXIO / no more data: rest is a hole; done
        }
        holeStart, err := seekHole(in, dataStart)
        if err != nil { holeStart = size }
        if _, err := in.Seek(dataStart, io.SeekStart); err != nil { return err }
        if _, err := out.Seek(dataStart, io.SeekStart); err != nil { return err }
        if _, err := io.CopyN(out, in, holeStart-dataStart); err != nil { return err }
        pos = holeStart
    }
    if err := out.Truncate(size); err != nil { return err } // preserve a trailing hole
    return out.Sync()
}
```

Check whether `golang.org/x/sys` is already a transitive dependency (likely, via `x/crypto/ssh`) before adding a direct `go.mod` entry.

## Files touched

- `vm/manager.go` — `copyFile` rewritten.
- `vm/manager_test.go` (or new `vm/copyfile_test.go`) — tests using a manually created sparse file (via `os.File.Truncate` past written data, or `unix.Fallocate` with a punched hole) asserting: (a) the copied file's content matches the original exactly, (b) the copied file's actual disk usage (via `Sys().(*syscall.Stat_t).Blocks`, not apparent size) is significantly smaller than nominal size when the source was sparse.

## Non-goals

- Do not change `copyFile`'s permission-copying behavior in this pass unless trivial to bundle (`os.Chmod(dst, srcInfo.Mode())` was also flagged separately — fine to include if it stays a one-line addition, but keep the diff focused on sparseness).
- Do not add the FICLONE/reflink fast-path in the first pass unless hole-skipping alone proves insufficient on real hardware.

## Success criteria

- Copying a sparse rootfs image results in a destination file with proportionally smaller on-disk usage (verified via block count, not apparent size).
- `go test ./vm/...` passes, including the new sparse-copy correctness/size tests.
- Manual `fcvm start` on real hardware shows reduced disk usage under `~/.fcvm/` for the copied rootfs relative to before the change.
