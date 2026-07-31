// Command fcvm-guest-agent listens on AF_VSOCK for host commands and streams
// stdout/stderr back to the host over a reverse vsock connection.
package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/pminnebach/fcvm/vsock"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fcvm-guest-agent: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ln, err := listenVsock(vsock.CmdPort)
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Fprintf(os.Stderr, "fcvm-guest-agent: listening on vsock port %d\n", vsock.CmdPort)

	for {
		conn, err := ln.Accept()
		if err != nil {
			return fmt.Errorf("accept: %w", err)
		}
		go handle(conn)
	}
}

func handle(conn net.Conn) {
	defer conn.Close()
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "fcvm-guest-agent: read command: %v\n", err)
		return
	}
	cmdLine := strings.TrimRight(line, "\r\n")
	if cmdLine == "" {
		return
	}

	out, err := dialVsock(vsock.HostCID, vsock.OutPort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fcvm-guest-agent: dial host output: %v\n", err)
		return
	}
	defer out.Close()

	code := runCommand(cmdLine, out)
	if _, err := io.WriteString(out, vsock.FormatExit(code)); err != nil {
		fmt.Fprintf(os.Stderr, "fcvm-guest-agent: write EXIT: %v\n", err)
	}
}

func runCommand(cmdline string, out io.Writer) int {
	cmd := exec.Command("/bin/sh", "-c", cmdline)
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		if status, ok := ee.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus()
		}
		return 1
	}
	fmt.Fprintf(out, "fcvm-guest-agent: %v\n", err)
	return 1
}

func listenVsock(port uint32) (net.Listener, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}
	sa := &unix.SockaddrVM{CID: unix.VMADDR_CID_ANY, Port: port}
	if err := unix.Bind(fd, sa); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if err := unix.Listen(fd, 128); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "vsock-listen")
	ln, err := net.FileListener(f)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	return ln, nil
}

func dialVsock(cid, port uint32) (net.Conn, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}
	sa := &unix.SockaddrVM{CID: cid, Port: port}
	if err := unix.Connect(fd, sa); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "vsock-dial")
	conn, err := net.FileConn(f)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	return conn, nil
}
