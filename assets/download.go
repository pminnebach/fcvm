package assets

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func DownloadFile(url, dest string) error {
	if err := EnsureDir(filepath.Dir(dest)); err != nil {
		return err
	}
	resp, err := http.Get(url) //nolint:noctx // ponytail: simple asset fetch
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, dest)
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

func LatestFirecrackerRelease() (string, error) {
	req, _ := http.NewRequest(http.MethodHead, "https://github.com/firecracker-microvm/firecracker/releases/latest", nil)
	resp, err := http.DefaultClient.Do(req)
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

func DownloadFirecracker(destDir string) (string, error) {
	ver, err := LatestFirecrackerRelease()
	if err != nil {
		return "", err
	}
	arch := firecrackerArch()
	url := fmt.Sprintf("https://github.com/firecracker-microvm/firecracker/releases/download/%s/firecracker-%s-%s.tgz", ver, ver, arch)
	tgz := filepath.Join(destDir, "firecracker.tgz")
	if err := DownloadFile(url, tgz); err != nil {
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

func DownloadKernel(url, dest string) error {
	return DownloadFile(url, dest)
}

func DownloadRootfs(url, dest string) error {
	if strings.HasSuffix(url, ".squashfs") || strings.Contains(url, "squashfs") {
		sqDest := dest + ".squashfs"
		if err := DownloadFile(url, sqDest); err != nil {
			return err
		}
		return SquashfsToExt4(sqDest, dest)
	}
	return DownloadFile(url, dest)
}

func SquashfsToExt4(squashfs, ext4Path string) error {
	dir, err := os.MkdirTemp("", "fcvm-unsquash-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	root := filepath.Join(dir, "root")
	if out, err := exec.Command("unsquashfs", "-d", root, squashfs).CombinedOutput(); err != nil {
		return fmt.Errorf("unsquashfs: %s: %w", out, err)
	}
	if err := InjectHooks(root); err != nil {
		return err
	}
	if err := os.Remove(ext4Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if out, err := exec.Command("truncate", "-s", "1G", ext4Path).CombinedOutput(); err != nil {
		return fmt.Errorf("truncate: %s: %w", out, err)
	}
	if out, err := exec.Command("mkfs.ext4", "-d", root, "-F", ext4Path).CombinedOutput(); err != nil {
		return fmt.Errorf("mkfs.ext4: %s: %w", out, err)
	}
	return nil
}

func DownloadJailerBuild(destDir string) error {
	ver, err := LatestFirecrackerRelease()
	if err != nil {
		return err
	}
	cloneDir := filepath.Join(destDir, "firecracker-src")
	if _, err := os.Stat(cloneDir); os.IsNotExist(err) {
		if out, err := exec.Command("git", "clone", "--depth", "1", "--branch", ver,
			"https://github.com/firecracker-microvm/firecracker.git", cloneDir).CombinedOutput(); err != nil {
			return fmt.Errorf("git clone: %s: %w", out, err)
		}
	}
	arch := runtime.GOARCH
	if out, err := exec.Command("bash", "-c", "cd "+cloneDir+" && tools/devtool build --release -- -l gnu").CombinedOutput(); err != nil {
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
