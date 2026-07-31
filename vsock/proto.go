// Package vsock holds Firecracker vsock path/port constants and the
// host↔guest command protocol shared by the host client and guest agent.
package vsock

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// GuestCID is the Firecracker guest_cid for the virtio-vsock device.
	GuestCID = 3
	// HostCID is the well-known host context ID (VMADDR_CID_HOST).
	HostCID = 2
	// CmdPort is the AF_VSOCK port the guest agent listens on for commands.
	CmdPort = 5252
	// OutPort is the port the guest dials on the host to stream command output.
	OutPort = 5253
	// UDSName is the relative AF_UNIX path inside the jailer chroot.
	UDSName = "vsock.sock"
	// DeviceID is the Firecracker vsock device id.
	DeviceID = "1"
)

// OutputUDSPath is the host AF_UNIX path Firecracker uses for guest-initiated
// connections to OutPort: "<uds_path>_<port>".
func OutputUDSPath(udsPath string) string {
	return fmt.Sprintf("%s_%d", udsPath, OutPort)
}

// FormatExit writes the protocol trailer the guest sends after command output.
func FormatExit(code int) string {
	return fmt.Sprintf("EXIT %d\n", code)
}

// ParseExitLine reports whether line is an EXIT trailer and returns its code.
func ParseExitLine(line string) (code int, ok bool) {
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, "EXIT ") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[5:]))
	if err != nil {
		return 0, false
	}
	return n, true
}
