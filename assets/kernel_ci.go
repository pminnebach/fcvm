package assets

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const firecrackerS3 = "https://s3.amazonaws.com/spec.ccfc.min"

var (
	s3PrefixRe  = regexp.MustCompile(`<Prefix>([^<]+)</Prefix>`)
	s3KeyRe     = regexp.MustCompile(`<Key>([^<]+)</Key>`)
	ciPrefixRe  = regexp.MustCompile(`^firecracker-ci/[0-9]{8}-[^/]+/$`)
	kernelKeyRe = regexp.MustCompile(`vmlinux-([0-9]+)\.([0-9]+)\.([0-9]{1,3})$`)
)

func parseS3Prefixes(xml string) []string {
	matches := s3PrefixRe.FindAllStringSubmatch(xml, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

func parseS3Keys(xml string) []string {
	matches := s3KeyRe.FindAllStringSubmatch(xml, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

func latestCIPrefix(prefixes []string) string {
	var ci []string
	for _, p := range prefixes {
		if ciPrefixRe.MatchString(p) {
			ci = append(ci, p)
		}
	}
	if len(ci) == 0 {
		return ""
	}
	sort.Strings(ci)
	return ci[len(ci)-1]
}

func latestKernelKey(prefix, arch string, keys []string) string {
	want := prefix + arch + "/"
	type kv struct {
		key string
		ver [3]int
	}
	var candidates []kv
	for _, key := range keys {
		if !strings.HasPrefix(key, want) {
			continue
		}
		base := strings.TrimPrefix(key, want)
		m := kernelKeyRe.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		ver := [3]int{}
		for i := 0; i < 3; i++ {
			ver[i], _ = strconv.Atoi(m[i+1])
		}
		candidates = append(candidates, kv{key: key, ver: ver})
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		for k := 0; k < 3; k++ {
			if candidates[i].ver[k] != candidates[j].ver[k] {
				return candidates[i].ver[k] < candidates[j].ver[k]
			}
		}
		return false
	})
	return candidates[len(candidates)-1].key
}

func fetchS3List(prefix string) (string, error) {
	url := firecrackerS3 + "?list-type=2&prefix=" + prefix
	if strings.HasSuffix(prefix, "/") {
		url += "&delimiter=/"
	}
	resp, err := http.Get(url) //nolint:noctx // ponytail: simple asset fetch
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("S3 list %s: HTTP %d", prefix, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func LatestFirecrackerKernelURL() (string, error) {
	arch := firecrackerArch()
	xml, err := fetchS3List("firecracker-ci/")
	if err != nil {
		return "", err
	}
	prefix := latestCIPrefix(parseS3Prefixes(xml))
	if prefix == "" {
		return "", fmt.Errorf("no firecracker-ci prefix found")
	}
	xml, err = fetchS3List(prefix + arch + "/vmlinux-")
	if err != nil {
		return "", err
	}
	key := latestKernelKey(prefix, arch, parseS3Keys(xml))
	if key == "" {
		return "", fmt.Errorf("no vmlinux for %s in %s", arch, prefix)
	}
	return firecrackerS3 + "/" + key, nil
}
