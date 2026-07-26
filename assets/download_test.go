package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
