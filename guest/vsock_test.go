package guest

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pminnebach/fcvm/vsock"
)

func TestDialGuestHandshake(t *testing.T) {
	dir := t.TempDir()
	uds := filepath.Join(dir, "vsock.sock")
	ln, err := net.Listen("unix", uds)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		line, _ := bufio.NewReader(c).ReadString('\n')
		if !strings.HasPrefix(line, "CONNECT ") {
			return
		}
		fmt.Fprintf(c, "OK 1073741824\n")
		io.Copy(io.Discard, c)
	}()

	conn, err := DialGuest(uds, vsock.CmdPort)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
}

func TestVsockExecSplitChannel(t *testing.T) {
	dir := t.TempDir()
	uds := filepath.Join(dir, "vsock.sock")

	// Fake Firecracker: accept CONNECT, then forward bytes; also accept the
	// guest's reverse connection by bridging to our "guest" logic inline.
	fcLn, err := net.Listen("unix", uds)
	if err != nil {
		t.Fatal(err)
	}
	defer fcLn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		cmdConn, err := fcLn.Accept()
		if err != nil {
			return
		}
		defer cmdConn.Close()
		br := bufio.NewReader(cmdConn)
		line, err := br.ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "CONNECT ") {
			return
		}
		fmt.Fprintf(cmdConn, "OK 1\n")
		cmdLine, err := br.ReadString('\n')
		if err != nil {
			return
		}
		cmdLine = strings.TrimRight(cmdLine, "\r\n")

		// Guest side of reverse channel: connect to output UDS.
		outPath := vsock.OutputUDSPath(uds)
		deadline := time.Now().Add(2 * time.Second)
		var out net.Conn
		for time.Now().Before(deadline) {
			out, err = net.Dial("unix", outPath)
			if err == nil {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if out == nil {
			return
		}
		defer out.Close()
		fmt.Fprintf(out, "ran:%s\n", cmdLine)
		io.WriteString(out, vsock.FormatExit(0))
	}()

	var buf bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := VsockExec(ctx, uds, "echo hi", &buf); err != nil {
		t.Fatal(err)
	}
	<-done
	if got := buf.String(); got != "ran:echo hi\n" {
		t.Fatalf("got %q", got)
	}
	_ = os.Remove(vsock.OutputUDSPath(uds))
}
