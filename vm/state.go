package vm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type MountState struct {
	Host   string `json:"host"`
	Guest  string `json:"guest"`
	Method string `json:"method"` // nfs, block
	Device string `json:"device,omitempty"`
}

type State struct {
	ID          string            `json:"id"`
	PID         int               `json:"pid"`
	SocketPath  string            `json:"socket_path"`
	TapDev      string            `json:"tap_dev"`
	GuestIP     string            `json:"guest_ip"`
	GuestMAC    string            `json:"guest_mac"`
	SSHKey      string            `json:"ssh_key"`
	ChrootDir   string            `json:"chroot_dir"`
	LogPath     string            `json:"log_path"`
	StartedAt   time.Time         `json:"started_at"`
	Mounts      []MountState      `json:"mounts,omitempty"`
	BlockImages []string          `json:"block_images,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

func statePath(stateDir, id string) string {
	return filepath.Join(stateDir, "vms", id, "state.json")
}

func LoadState(stateDir, id string) (*State, error) {
	data, err := os.ReadFile(statePath(stateDir, id))
	if err != nil {
		return nil, fmt.Errorf("load state for %q: %w", id, err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state for %q: %w", id, err)
	}
	return &s, nil
}

func SaveState(stateDir string, s *State) error {
	dir := filepath.Join(stateDir, "vms", s.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "state.json"), data, 0o644)
}

func RemoveState(stateDir, id string) error {
	return os.RemoveAll(filepath.Join(stateDir, "vms", id))
}

func ListVMDirIDs(stateDir string) ([]string, error) {
	dir := filepath.Join(stateDir, "vms")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func ListStates(stateDir string) ([]State, error) {
	dir := filepath.Join(stateDir, "vms")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []State
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := LoadState(stateDir, e.Name())
		if err != nil {
			continue
		}
		out = append(out, *s)
	}
	return out, nil
}
