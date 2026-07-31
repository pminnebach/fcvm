package assets

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadFileVerifiesChecksum(t *testing.T) {
	payload := []byte("firecracker binary stand-in")
	sum := sha256.Sum256(payload)
	good := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "asset")

	if err := DownloadFile(context.Background(), srv.URL, dest, FetchOptions{SHA256: good, Insecure: true}); err != nil {
		t.Fatalf("matching checksum should succeed: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("downloaded content = %q, %v", got, err)
	}

	// A failed verification must not leave the artifact behind: fcvm execs
	// these files as root.
	bad := filepath.Join(dir, "bad")
	err = DownloadFile(context.Background(), srv.URL, bad, FetchOptions{SHA256: "00" + good[2:], Insecure: true})
	if err == nil {
		t.Fatal("expected a checksum mismatch error")
	}
	if _, statErr := os.Stat(bad); statErr == nil {
		t.Fatal("mismatched download was left on disk")
	}
	if _, statErr := os.Stat(bad + ".tmp"); statErr == nil {
		t.Fatal("partial download was left on disk")
	}
}

func TestDownloadFileRejectsPlainHTTP(t *testing.T) {
	err := DownloadFile(context.Background(), "http://example.invalid/x", filepath.Join(t.TempDir(), "x"), FetchOptions{})
	if err == nil {
		t.Fatal("plain http should be refused without --insecure")
	}
}

func TestCheckScheme(t *testing.T) {
	if err := checkScheme("https://example.com/x", false); err != nil {
		t.Fatalf("https should be allowed: %v", err)
	}
	if err := checkScheme("http://example.com/x", true); err != nil {
		t.Fatalf("http with insecure should be allowed: %v", err)
	}
	if err := checkScheme("file:///etc/passwd", false); err == nil {
		t.Fatal("non-http scheme should be refused")
	}
}

func TestFetchSHA256(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("382a02a869e4d6d5cb14c40577f9545e8458021ea8b0b2d3fc10ec14d9c242e6  firecracker-v1.16.1-x86_64.tgz\n"))
	}))
	defer srv.Close()
	got, err := fetchSHA256(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got != "382a02a869e4d6d5cb14c40577f9545e8458021ea8b0b2d3fc10ec14d9c242e6" {
		t.Fatalf("digest = %q", got)
	}
}

func TestNormalizeGuestAgentVersion(t *testing.T) {
	ver, tag, err := normalizeGuestAgentVersion("1.1.0")
	if err != nil || ver != "1.1.0" || tag != "v1.1.0" {
		t.Fatalf("got %q %q %v", ver, tag, err)
	}
	ver, tag, err = normalizeGuestAgentVersion("v1.2.3")
	if err != nil || ver != "1.2.3" || tag != "v1.2.3" {
		t.Fatalf("got %q %q %v", ver, tag, err)
	}
	for _, bad := range []string{"", "dev", "6bdb790", "latest"} {
		if _, _, err := normalizeGuestAgentVersion(bad); err == nil {
			t.Fatalf("version %q should be rejected", bad)
		}
	}
}

func guestAgentTestArchive(t *testing.T, agent []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "fcvm-guest-agent", Mode: 0o755, Size: int64(len(agent))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(agent); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDownloadGuestAgentRelease(t *testing.T) {
	agent := []byte("guest-agent-bytes")
	archive := guestAgentTestArchive(t, agent)
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])

	mux := http.NewServeMux()
	mux.HandleFunc("/release.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		w.Write(archive)
	})
	mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s  release.tar.gz\n", digest)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "bin", "fcvm-guest-agent")
	if err := downloadGuestAgentReleaseOpts(context.Background(), srv.URL+"/release.tar.gz", srv.URL+"/checksums.txt", dest, FetchOptions{Insecure: true}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, agent) {
		t.Fatalf("dest = %q, %v", got, err)
	}
	fi, err := os.Stat(dest)
	if err != nil || fi.Mode()&0o111 == 0 {
		t.Fatalf("want executable, mode=%v err=%v", fi.Mode(), err)
	}
}

func TestDownloadGuestAgentReleaseMissingChecksum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	err := downloadGuestAgentRelease(context.Background(), srv.URL+"/a.tar.gz", srv.URL+"/checksums.txt", filepath.Join(t.TempDir(), "agent"))
	if err == nil || !strings.Contains(err.Error(), "checksum unavailable") {
		t.Fatalf("err = %v, want checksum unavailable", err)
	}
}

func TestDownloadGuestAgentRejectsDev(t *testing.T) {
	err := DownloadGuestAgent(context.Background(), "dev", filepath.Join(t.TempDir(), "agent"))
	if err == nil || !strings.Contains(err.Error(), "--url") {
		t.Fatalf("err = %v, want --url hint", err)
	}
}

func TestDownloadGuestAgentURL(t *testing.T) {
	payload := []byte("custom-agent")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(payload)
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "fcvm-guest-agent")
	if err := DownloadGuestAgentURL(context.Background(), srv.URL, dest, FetchOptions{Insecure: true}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("got %q, %v", got, err)
	}
	fi, err := os.Stat(dest)
	if err != nil || fi.Mode()&0o111 == 0 {
		t.Fatalf("want executable, mode=%v err=%v", fi.Mode(), err)
	}
}
