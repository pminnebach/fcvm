package config

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// foreignOwnedDirConfig writes a Directory config (mode 0644) whose owner is
// neither root nor the returned invoking uid, and returns the dir, file path,
// that owner uid and a different invoking uid.
func foreignOwnedDirConfig(t *testing.T) (cwd, path string, owner, invoking int) {
	t.Helper()
	cwd = t.TempDir()
	path = writeConfigFile(t, cwd, "vcpu-count: 2\n")
	if os.Getuid() == 0 {
		owner = 54321
		if err := os.Chown(path, owner, -1); err != nil {
			t.Skipf("cannot chown: %v", err)
		}
	} else {
		owner = os.Getuid()
	}
	return cwd, path, owner, owner + 1
}

func resolveDir(t *testing.T, cwd string, uid int) error {
	t.Helper()
	_, _, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: t.TempDir(), UID: uid})
	return err
}

func TestDirectoryConfigWithDifferentOwnerRefused(t *testing.T) {
	cwd, path, owner, invoking := foreignOwnedDirConfig(t)
	err := resolveDir(t, cwd, invoking)
	if err == nil {
		t.Fatal("want error for a Directory config owned by another uid")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "owner") ||
		!strings.Contains(err.Error(), strconv.Itoa(owner)) {
		t.Fatalf("error %q should name the file and the owner", err)
	}
}

func TestDirectoryConfigGroupOrWorldWritableRefused(t *testing.T) {
	for name, mode := range map[string]os.FileMode{"group": 0o664, "world": 0o646} {
		t.Run(name, func(t *testing.T) {
			cwd := t.TempDir()
			path := writeConfigFile(t, cwd, "vcpu-count: 2\n")
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			err := resolveDir(t, cwd, os.Getuid())
			if err == nil {
				t.Fatal("want error for a writable Directory config")
			}
			if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), name+"-writable") {
				t.Fatalf("error %q should name the file and %s-writable", err, name)
			}
		})
	}
}

func TestDirectoryConfigOwnedByInvokingUserAccepted(t *testing.T) {
	cwd := t.TempDir()
	writeConfigFile(t, cwd, "vcpu-count: 2\n")
	if err := resolveDir(t, cwd, os.Getuid()); err != nil {
		t.Fatal(err)
	}
}

func TestDirectoryConfigOwnedByRootAccepted(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("needs root to own the file as root")
	}
	cwd := t.TempDir()
	writeConfigFile(t, cwd, "vcpu-count: 2\n")
	if err := resolveDir(t, cwd, 1234); err != nil {
		t.Fatal(err)
	}
}

func TestUserAndExplicitConfigNotTrustChecked(t *testing.T) {
	cwd, path, _, invoking := foreignOwnedDirConfig(t)
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	// As the User config (cwd is home) and as --config: no check.
	if _, _, err := Resolve(viper.New(), ResolveInput{Cwd: cwd, Home: cwd, UID: invoking}); err != nil {
		t.Fatalf("User config was checked: %v", err)
	}
	if _, _, err := Resolve(viper.New(), ResolveInput{ConfigFile: path, Cwd: cwd, Home: t.TempDir(), UID: invoking}); err != nil {
		t.Fatalf("--config file was checked: %v", err)
	}
}
