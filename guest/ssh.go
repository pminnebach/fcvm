package guest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func WaitSSH(host, keyPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := exec.Command("ssh",
			"-i", keyPath,
			"-o", "StrictHostKeyChecking=no",
			"-o", "UserKnownHostsFile=/dev/null",
			"-o", "ConnectTimeout=2",
			"-o", "BatchMode=yes",
			"-o", "PasswordAuthentication=no",
			"root@"+host, "true",
		).Run(); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("ssh to %s not ready within %s", host, timeout)
}

func Exec(host, keyPath string, args []string) error {
	cmdArgs := []string{
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"-o", "PasswordAuthentication=no",
		"root@" + host,
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("ssh", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func Shell(host, keyPath string) error {
	cmd := exec.Command("ssh",
		"-i", keyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"-o", "PasswordAuthentication=no",
		"-t",
		"root@"+host,
	)
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
