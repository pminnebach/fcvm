package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveGuestAgent returns the path to the fcvm-guest-agent binary.
// Prefers GuestAgentBin when it exists, otherwise a sibling of the running
// executable named fcvm-guest-agent.
func ResolveGuestAgent(c Config) (string, error) {
	if c.GuestAgentBin != "" {
		if _, err := os.Stat(c.GuestAgentBin); err == nil {
			return c.GuestAgentBin, nil
		}
	}
	exe, err := os.Executable()
	if err == nil {
		sibling := filepath.Join(filepath.Dir(exe), "fcvm-guest-agent")
		if _, err := os.Stat(sibling); err == nil {
			return sibling, nil
		}
	}
	if c.GuestAgentBin != "" {
		return "", fmt.Errorf("guest agent binary not found at %q (build with: go build -buildvcs=false -o %s ./guest/agent)",
			c.GuestAgentBin, c.GuestAgentBin)
	}
	return "", fmt.Errorf("guest agent binary not found (set guest-agent-bin or place fcvm-guest-agent next to fcvm)")
}
