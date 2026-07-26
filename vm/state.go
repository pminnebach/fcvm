package vm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pminnebach/fcvm/network"
)

type MountState struct {
	Host     string `json:"host"`
	Guest    string `json:"guest"`
	Method   string `json:"method"` // nfs, block
	Device   string `json:"device,omitempty"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

const (
	NetworkModeTAP = "tap"
	NetworkModeCNI = "cni"
)

type State struct {
	ID          string            `json:"id"`
	Index       int               `json:"index"`
	PID         int               `json:"pid"`
	PIDStart    uint64            `json:"pid_start,omitempty"` // /proc/<pid>/stat starttime, guards against PID reuse
	SocketPath  string            `json:"socket_path"`
	NetworkMode string            `json:"network_mode,omitempty"` // tap (default) or cni
	CNINetwork  string            `json:"cni_network,omitempty"`
	TapDev      string            `json:"tap_dev"`
	HostIface   string            `json:"host_iface,omitempty"`
	GuestSubnet string            `json:"guest_subnet,omitempty"`
	GuestIP     string            `json:"guest_ip"`
	GuestMAC    string            `json:"guest_mac"`
	SSHKey      string            `json:"ssh_key"`
	ChrootDir   string            `json:"chroot_dir"`
	LogPath     string            `json:"log_path"`
	JailerUID   int               `json:"jailer_uid,omitempty"`
	JailerGID   int               `json:"jailer_gid,omitempty"`
	StartedAt   time.Time         `json:"started_at"`
	Mounts      []MountState      `json:"mounts,omitempty"`
	BlockImages []string          `json:"block_images,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
}

func (s *State) IsCNI() bool {
	return s != nil && s.NetworkMode == NetworkModeCNI
}

// IsRunning reports whether the recorded PID is alive and still belongs to
// this VM. PID alone is not enough: the kernel recycles PIDs, and signalling a
// recycled PID as root can hit an unrelated process.
func (s *State) IsRunning() bool {
	if s == nil || s.PID <= 0 {
		return false
	}
	start, err := procStartTime(s.PID)
	if err != nil {
		return false
	}
	// States written before pid_start existed fall back to liveness only.
	if s.PIDStart != 0 && s.PIDStart != start {
		return false
	}
	return true
}

// ClaimedIndices returns the network indices already taken by existing VMs.
// TAP device names are authoritative when present so that states written
// before the index field existed still claim their slot.
func ClaimedIndices(states []State) map[int]bool {
	claimed := make(map[int]bool, len(states))
	for _, s := range states {
		if n, ok := indexFromTapDev(s.TapDev); ok {
			claimed[n] = true
			continue
		}
		claimed[s.Index] = true
	}
	return claimed
}

func indexFromTapDev(dev string) (int, bool) {
	return network.IndexFromTapDev(dev)
}

// procStartTime reads field 22 of /proc/<pid>/stat, the process start time in
// clock ticks since boot. Together with the PID it identifies a process
// uniquely for as long as the host is up.
func procStartTime(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	// comm (field 2) is parenthesised and may contain spaces, so split after it.
	close := bytes.LastIndexByte(data, ')')
	if close < 0 {
		return 0, fmt.Errorf("parse /proc/%d/stat", pid)
	}
	fields := strings.Fields(string(data[close+1:]))
	const startTimeOffset = 19 // field 22 overall, first field here is 3
	if len(fields) <= startTimeOffset {
		return 0, fmt.Errorf("parse /proc/%d/stat: too few fields", pid)
	}
	return strconv.ParseUint(fields[startTimeOffset], 10, 64)
}

// ValidateID rejects VM ids that are not a single safe path component.
// The id becomes a directory name, a jailer chroot path, and an NFS export
// filename, all created and removed as root, so keep the alphabet narrow.
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("vm id must not be empty")
	}
	if len(id) > maxIDLen {
		return fmt.Errorf("vm id %q is longer than %d characters", id, maxIDLen)
	}
	if id == "." || id == ".." || filepath.Base(id) != id {
		return fmt.Errorf("vm id %q must be a single path component", id)
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("vm id %q may only contain letters, digits, '-', '_' and '.'", id)
		}
	}
	return nil
}

const maxIDLen = 64

func statePath(stateDir, id string) string {
	return filepath.Join(stateDir, "vms", id, "state.json")
}

// VMDir returns the per-VM state directory, after validating the id.
func VMDir(stateDir, id string) (string, error) {
	if err := ValidateID(id); err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "vms", id), nil
}

func LoadState(stateDir, id string) (*State, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
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
	dir, err := VMDir(stateDir, s.ID)
	if err != nil {
		return err
	}
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
	dir, err := VMDir(stateDir, id)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
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
