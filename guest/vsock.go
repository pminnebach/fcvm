package guest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/pminnebach/fcvm/vsock"
)

// DialGuest connects to the Firecracker vsock UDS and requests a host-initiated
// connection to the guest AF_VSOCK port (CONNECT handshake).
func DialGuest(udsPath string, port uint32) (net.Conn, error) {
	conn, err := net.Dial("unix", udsPath)
	if err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(conn, "CONNECT %d\n", port); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("send CONNECT: %w", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("read CONNECT ack: %w", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(line), "OK ") {
		_ = conn.Close()
		return nil, fmt.Errorf("unexpected CONNECT reply %q", strings.TrimSpace(line))
	}
	return conn, nil
}

// ListenOutput listens on the host AF_UNIX path used for guest-initiated
// connections to the output port. The socket is mode 0666 so the jailed
// Firecracker process (non-root) can connect.
func ListenOutput(udsPath string) (net.Listener, error) {
	path := vsock.OutputUDSPath(udsPath)
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	// ponytail: world-writable UDS in the jail chroot; Fine while only the
	// jailer uid can reach that directory. Tighten with chown(jailer) if the
	// chroot is ever shared.
	if err := os.Chmod(path, 0o666); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return ln, nil
}

// VsockExec runs command in the guest over the split-channel vsock protocol
// and writes stdout/stderr to out. A non-zero guest exit becomes *vsock.ExitError.
func VsockExec(ctx context.Context, udsPath string, command string, out io.Writer) error {
	if udsPath == "" {
		return fmt.Errorf("vsock uds path is empty")
	}
	if command == "" {
		return fmt.Errorf("command is empty")
	}

	ln, err := ListenOutput(udsPath)
	if err != nil {
		return fmt.Errorf("listen for guest output: %w", err)
	}
	defer ln.Close()
	defer os.Remove(vsock.OutputUDSPath(udsPath))

	type acceptResult struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		conn, err := ln.Accept()
		accepted <- acceptResult{conn, err}
	}()

	cmdConn, err := DialGuest(udsPath, vsock.CmdPort)
	if err != nil {
		return err
	}
	defer cmdConn.Close()

	if _, err := fmt.Fprintf(cmdConn, "%s\n", command); err != nil {
		return fmt.Errorf("send command: %w", err)
	}
	if uc, ok := cmdConn.(*net.UnixConn); ok {
		_ = uc.CloseWrite()
	}

	var ar acceptResult
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ar = <-accepted:
	}
	if ar.err != nil {
		return fmt.Errorf("accept guest output: %w", ar.err)
	}
	defer ar.conn.Close()

	done := make(chan error, 1)
	go func() {
		done <- vsock.CopyOutput(out, ar.conn)
	}()
	select {
	case <-ctx.Done():
		_ = ar.conn.Close()
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// WaitVsock retries DialGuest until the guest agent accepts or the timeout elapses.
func WaitVsock(ctx context.Context, udsPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn, err := DialGuest(udsPath, vsock.CmdPort)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if last == nil {
		last = fmt.Errorf("timeout")
	}
	return fmt.Errorf("guest vsock not ready after %s: %w", timeout, last)
}
