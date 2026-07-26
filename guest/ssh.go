package guest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type KeyPair struct {
	PrivateKeyPath string
	PublicKey      string
}

func LoadOrCreateKey(path string) (*KeyPair, error) {
	if data, err := os.ReadFile(path); err == nil {
		pub, err := sshPublicFromPrivate(data)
		if err != nil {
			return nil, err
		}
		return &KeyPair{PrivateKeyPath: path, PublicKey: pub}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	pemBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(pemBlock), 0o600); err != nil {
		return nil, err
	}
	pub, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		return nil, err
	}
	return &KeyPair{
		PrivateKeyPath: path,
		PublicKey:      string(ssh.MarshalAuthorizedKey(pub)),
	}, nil
}

func sshPublicFromPrivate(pemData []byte) (string, error) {
	signer, err := ssh.ParsePrivateKey(pemData)
	if err != nil {
		return "", err
	}
	return string(ssh.MarshalAuthorizedKey(signer.PublicKey())), nil
}

// sshOpts are the connection options shared by every helper here.
func sshOpts(keyPath string) []string {
	return []string{
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"-o", "PasswordAuthentication=no",
	}
}

// WaitSSH polls until the guest accepts an SSH connection, the timeout
// expires, or ctx is cancelled.
func WaitSSH(ctx context.Context, host, keyPath string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		args := append(sshOpts(keyPath), "-o", "ConnectTimeout=2", "root@"+host, "true")
		if err := exec.CommandContext(ctx, "ssh", args...).Run(); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("ssh to %s not ready within %s: %w", host, timeout, ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

// ShellQuote wraps s so a remote shell treats it as a single literal word.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// RemoteCommand joins argv into one command string for the remote shell. ssh
// concatenates its trailing arguments with spaces and hands the result to a
// shell, so anything not quoted here is re-split on the other side.
func RemoteCommand(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = ShellQuote(a)
	}
	return strings.Join(quoted, " ")
}

func Exec(host, keyPath string, args []string) error {
	cmdArgs := append(sshOpts(keyPath), "root@"+host, RemoteCommand(args))
	cmd := exec.Command("ssh", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// WriteFile writes content to remotePath in the guest, creating its directory.
func WriteFile(host, keyPath, remotePath, content string) error {
	remote := fmt.Sprintf("mkdir -p %s && cat > %s",
		ShellQuote(filepath.Dir(remotePath)), ShellQuote(remotePath))
	cmd := exec.Command("ssh", append(sshOpts(keyPath), "root@"+host, remote)...)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func Shell(host, keyPath string) error {
	cmd := exec.Command("ssh", append(sshOpts(keyPath), "-t", "root@"+host)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func TailFollow(path string) error {
	cmd := exec.Command("tail", "-f", path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
