package vm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pminnebach/fcvm/config"
)

// A VM's directory holds up to three records:
//
//	config.json   the VM configuration, written once at creation
//	state.json    runtime facts, present only while the VM runs (or crashed)
//	stopped.json  written by a deliberate stop, removed on the next start
//
// Status is derived from these plus the live process; it is never stored.
const (
	configFile  = "config.json"
	stoppedFile = "stopped.json"
)

// VMConfig is the shape a VM was created with. It is fixed at creation and
// reused on every start, so a restart cannot drift from how the disk was
// provisioned.
type VMConfig struct {
	ID    string `json:"id"`
	Index int    `json:"index"`
	// Config is the effective configuration at creation: machine knobs,
	// network mode, mounts, env, vsock and jailer settings.
	Config config.Config `json:"config"`
	// Resolved TAP addressing. Empty in CNI mode, where the address comes from
	// the CNI result at each start.
	TapDev      string    `json:"tap_dev,omitempty"`
	TapIP       string    `json:"tap_ip,omitempty"`
	GuestIP     string    `json:"guest_ip,omitempty"`
	GuestMAC    string    `json:"guest_mac,omitempty"`
	GuestSubnet string    `json:"guest_subnet,omitempty"`
	JailerUID   int       `json:"jailer_uid,omitempty"`
	JailerGID   int       `json:"jailer_gid,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// IsCNI reports whether the VM gets its address from CNI.
func (c *VMConfig) IsCNI() bool { return c != nil && c.Config.Network.CNINetwork != "" }

type stopRecord struct {
	StoppedAt time.Time `json:"stopped_at"`
}

// Status is where a VM is in its lifecycle.
type Status string

const (
	StatusRunning Status = "running"
	StatusStopped Status = "stopped" // deliberately stopped by the user
	StatusCrashed Status = "crashed" // process gone without a stop
	StatusLegacy  Status = "legacy"  // created before configuration records existed
)

// Info is a VM together with the records that determine its status.
type Info struct {
	ID        string
	Status    Status
	Config    *VMConfig // nil for legacy VMs
	State     *State    // nil once the runtime state is removed
	StoppedAt time.Time // zero unless there is a stop record
}

func vmFile(stateDir, id, name string) (string, error) {
	dir, err := VMDir(stateDir, id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// SaveVMConfig writes the configuration record. It refuses to overwrite one:
// the record is immutable.
func SaveVMConfig(stateDir string, c *VMConfig) error {
	path, err := vmFile(stateDir, c.ID, configFile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("write configuration for %q: %w", c.ID, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func LoadVMConfig(stateDir, id string) (*VMConfig, error) {
	path, err := vmFile(stateDir, id, configFile)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load configuration for %q: %w", id, err)
	}
	var c VMConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse configuration for %q: %w", id, err)
	}
	return &c, nil
}

func SaveStopRecord(stateDir, id string, at time.Time) error {
	path, err := vmFile(stateDir, id, stoppedFile)
	if err != nil {
		return err
	}
	data, err := json.Marshal(stopRecord{StoppedAt: at})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func loadStopRecord(stateDir, id string) (time.Time, error) {
	path, err := vmFile(stateDir, id, stoppedFile)
	if err != nil {
		return time.Time{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}, err
	}
	var r stopRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return time.Time{}, err
	}
	return r.StoppedAt, nil
}

func removeStopRecord(stateDir, id string) error {
	path, err := vmFile(stateDir, id, stoppedFile)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// removeRuntimeState removes only state.json, keeping the rest of the VM.
func removeRuntimeState(stateDir, id string) error {
	path, err := vmFile(stateDir, id, "state.json")
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Inspect classifies one VM from its records and the live process. A process
// that is alive always wins: a running VM is reported running whatever else is
// on disk.
func Inspect(stateDir, id string) (*Info, error) {
	if err := ValidateID(id); err != nil {
		return nil, err
	}
	info := &Info{ID: id}
	if cfg, err := LoadVMConfig(stateDir, id); err == nil {
		info.Config = cfg
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if st, err := LoadState(stateDir, id); err == nil {
		info.State = st
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if at, err := loadStopRecord(stateDir, id); err == nil {
		info.StoppedAt = at
	}
	if info.Config == nil && info.State == nil {
		return nil, fmt.Errorf("no such VM %q: %w", id, os.ErrNotExist)
	}
	switch {
	case info.State.IsRunning():
		info.Status = StatusRunning
	case info.Config == nil:
		info.Status = StatusLegacy
	case !info.StoppedAt.IsZero():
		info.Status = StatusStopped
	case info.State != nil:
		info.Status = StatusCrashed
	default:
		// Configuration only: the runtime state is gone and nothing was left
		// running, which is what a stopped VM looks like.
		info.Status = StatusStopped
	}
	return info, nil
}

// ListInfos inspects every VM directory. Directories with no records at all
// are orphans for cleanup to reclaim, not VMs.
func ListInfos(stateDir string) ([]Info, error) {
	ids, err := ListVMDirIDs(stateDir)
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, id := range ids {
		info, err := Inspect(stateDir, id)
		if err != nil {
			continue
		}
		out = append(out, *info)
	}
	return out, nil
}

// RunningState returns the runtime state of a running VM. For one that is not
// running it returns an error saying what state the VM is in and what to do
// about it, so exec and shell do not fail with a bare connection error.
func RunningState(stateDir, id string) (*State, error) {
	info, err := Inspect(stateDir, id)
	if err != nil {
		return nil, err
	}
	switch info.Status {
	case StatusRunning:
		return info.State, nil
	case StatusStopped:
		return nil, fmt.Errorf("VM %q is stopped; start it with `fcvm start %s`", id, id)
	case StatusCrashed:
		return nil, fmt.Errorf("VM %q crashed; start it again with `fcvm start %s`", id, id)
	default:
		return nil, fmt.Errorf("VM %q was created before VMs could be restarted and is not running; remove it with `fcvm cleanup %s`", id, id)
	}
}
