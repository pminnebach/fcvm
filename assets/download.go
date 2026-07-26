package assets

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pminnebach/fcvm/rootfs"
)

// httpClient bounds connect and stall time without capping total transfer
// time: kernel and rootfs images are large but a dead server must not hang
// the CLI forever.
var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	},
}

// FetchOptions controls verification for a single download.
type FetchOptions struct {
	// SHA256 is the expected hex digest. Empty means unverified, which is
	// allowed only over HTTPS and warns.
	SHA256 string
	// Insecure permits a plain http:// URL.
	Insecure bool
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func checkScheme(rawURL string, insecure bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse url %q: %w", rawURL, err)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if insecure {
			return nil
		}
		return fmt.Errorf("refusing plain http URL %q; pass --insecure to override", rawURL)
	default:
		return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
}

func get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: HTTP %d", rawURL, resp.StatusCode)
	}
	return resp, nil
}

// DownloadFile fetches url to dest, verifying the digest while streaming. On
// any failure the partial file is removed, so a failed verification can never
// leave a usable artifact behind.
func DownloadFile(ctx context.Context, rawURL, dest string, opts FetchOptions) error {
	if err := checkScheme(rawURL, opts.Insecure); err != nil {
		return err
	}
	if opts.SHA256 == "" {
		fmt.Fprintf(os.Stderr, "fcvm: warning: no checksum available for %s; cannot verify integrity\n", rawURL)
	}
	if err := EnsureDir(filepath.Dir(dest)); err != nil {
		return err
	}
	resp, err := get(ctx, rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, hasher), resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(tmp)
		return closeErr
	}
	if opts.SHA256 != "" {
		got := hex.EncodeToString(hasher.Sum(nil))
		if !strings.EqualFold(got, opts.SHA256) {
			os.Remove(tmp)
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", rawURL, opts.SHA256, got)
		}
	}
	return os.Rename(tmp, dest)
}

// fetchSHA256 reads a `sha256sum`-style file and returns the digest.
func fetchSHA256(ctx context.Context, rawURL string) (string, error) {
	resp, err := get(ctx, rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(body))
	if len(fields) == 0 || len(fields[0]) != 64 {
		return "", fmt.Errorf("unexpected checksum file at %s", rawURL)
	}
	return fields[0], nil
}

func firecrackerArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return runtime.GOARCH
	}
}

func LatestFirecrackerRelease(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead,
		"https://github.com/firecracker-microvm/firecracker/releases/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	resolveURL := loc
	if resolveURL == "" {
		resolveURL = finalURL
	}
	if resolveURL == "" {
		return "", fmt.Errorf("no redirect from releases/latest")
	}
	parts := strings.Split(strings.TrimSuffix(resolveURL, "/"), "/")
	tag := parts[len(parts)-1]
	if tag == "" || tag == "latest" {
		return "", fmt.Errorf("could not parse release tag from %q", resolveURL)
	}
	return tag, nil
}

// DownloadFirecracker fetches the release tarball and verifies it against the
// checksum published alongside it. These binaries are later executed as root,
// so an unverifiable download is a hard failure rather than a warning.
func DownloadFirecracker(ctx context.Context, destDir string) (string, error) {
	ver, err := LatestFirecrackerRelease(ctx)
	if err != nil {
		return "", err
	}
	arch := firecrackerArch()
	base := fmt.Sprintf("https://github.com/firecracker-microvm/firecracker/releases/download/%s/firecracker-%s-%s.tgz", ver, ver, arch)
	sum, err := fetchSHA256(ctx, base+".sha256.txt")
	if err != nil {
		return "", fmt.Errorf("firecracker %s checksum unavailable, refusing to install unverified binaries: %w", ver, err)
	}
	tgz := filepath.Join(destDir, "firecracker.tgz")
	if err := DownloadFile(ctx, base, tgz, FetchOptions{SHA256: sum}); err != nil {
		return "", err
	}
	if err := extractFirecrackerTarball(tgz, destDir, ver, arch); err != nil {
		return "", err
	}
	return ver, nil
}

func extractFirecrackerTarball(tgz, destDir, ver, arch string) error {
	f, err := os.Open(tgz)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		base := filepath.Base(hdr.Name)
		var destName string
		switch base {
		case "firecracker-" + ver + "-" + arch:
			destName = "firecracker"
		case "jailer-" + ver + "-" + arch:
			destName = "jailer"
		}
		if destName == "" {
			continue
		}
		dest := filepath.Join(destDir, destName)
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()
	}
	return nil
}

func DownloadKernel(ctx context.Context, rawURL, dest string, opts FetchOptions) error {
	return DownloadFile(ctx, rawURL, dest, opts)
}

func DownloadRootfs(ctx context.Context, rawURL, dest, size string, opts FetchOptions) error {
	if strings.HasSuffix(rawURL, ".squashfs") || strings.Contains(rawURL, "squashfs") {
		sqDest := dest + ".squashfs"
		if err := DownloadFile(ctx, rawURL, sqDest, opts); err != nil {
			return err
		}
		return SquashfsToExt4(sqDest, dest, size)
	}
	return DownloadFile(ctx, rawURL, dest, opts)
}

func SquashfsToExt4(squashfs, ext4Path, size string) error {
	dir, err := os.MkdirTemp("", "fcvm-unsquash-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	root := filepath.Join(dir, "root")
	if out, err := exec.Command("unsquashfs", "-d", root, squashfs).CombinedOutput(); err != nil {
		return fmt.Errorf("unsquashfs: %s: %w", out, err)
	}
	if err := rootfs.InjectHooks(root); err != nil {
		return err
	}
	if size == "" {
		size, err = rootfs.SizeForDir(root)
		if err != nil {
			return err
		}
	}
	return rootfs.MakeExt4(root, ext4Path, size)
}

func DownloadJailerBuild(ctx context.Context, destDir string) error {
	ver, err := LatestFirecrackerRelease(ctx)
	if err != nil {
		return err
	}
	cloneDir := filepath.Join(destDir, "firecracker-src")
	if _, err := os.Stat(cloneDir); os.IsNotExist(err) {
		if out, err := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", ver,
			"https://github.com/firecracker-microvm/firecracker.git", cloneDir).CombinedOutput(); err != nil {
			return fmt.Errorf("git clone: %s: %w", out, err)
		}
	}
	arch := runtime.GOARCH
	if out, err := exec.CommandContext(ctx, "bash", "-c",
		"cd "+cloneDir+" && tools/devtool build --release -- -l gnu").CombinedOutput(); err != nil {
		return fmt.Errorf("devtool build: %s: %w", out, err)
	}
	fcBin := filepath.Join(cloneDir, "build", "cargo_target", arch+"-unknown-linux-gnu", "release", "firecracker")
	jailerBin := filepath.Join(cloneDir, "build", "cargo_target", arch+"-unknown-linux-gnu", "release", "jailer")
	for _, pair := range [][2]string{{fcBin, "firecracker"}, {jailerBin, "jailer"}} {
		data, err := os.ReadFile(pair[0])
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destDir, pair[1]), data, 0o755); err != nil {
			return err
		}
	}
	return nil
}
