package assets

import "testing"

func TestLatestCIPrefix(t *testing.T) {
	xml := `<ListBucketResult>
<Prefix>firecracker-ci/20250101-abc/</Prefix>
<Prefix>firecracker-ci/20250201-def/</Prefix>
<Prefix>other/</Prefix>
</ListBucketResult>`
	got := latestCIPrefix(parseS3Prefixes(xml))
	want := "firecracker-ci/20250201-def/"
	if got != want {
		t.Fatalf("latestCIPrefix() = %q, want %q", got, want)
	}
}

func TestLatestKernelKey(t *testing.T) {
	prefix := "firecracker-ci/20250201-def/"
	arch := "x86_64"
	keys := []string{
		prefix + arch + "/vmlinux-6.1.100",
		prefix + arch + "/vmlinux-6.1.102",
		prefix + arch + "/vmlinux-6.1.101",
		prefix + "aarch64/vmlinux-6.1.999",
	}
	got := latestKernelKey(prefix, arch, keys)
	want := prefix + arch + "/vmlinux-6.1.102"
	if got != want {
		t.Fatalf("latestKernelKey() = %q, want %q", got, want)
	}
}

func TestLatestFirecrackerKernelURLShape(t *testing.T) {
	prefix := "firecracker-ci/20250201-def/"
	arch := "x86_64"
	key := latestKernelKey(prefix, arch, []string{prefix + arch + "/vmlinux-6.1.102"})
	url := firecrackerS3 + "/" + key
	want := "https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/20250201-def/x86_64/vmlinux-6.1.102"
	if url != want {
		t.Fatalf("URL = %q, want %q", url, want)
	}
}
