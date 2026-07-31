package vsock

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseExitLine(t *testing.T) {
	code, ok := ParseExitLine("EXIT 0\n")
	if !ok || code != 0 {
		t.Fatalf("got %d %v", code, ok)
	}
	code, ok = ParseExitLine("EXIT 42")
	if !ok || code != 42 {
		t.Fatalf("got %d %v", code, ok)
	}
	if _, ok := ParseExitLine("hello\n"); ok {
		t.Fatal("expected not ok")
	}
}

func TestCopyOutputStripsExit(t *testing.T) {
	var buf bytes.Buffer
	err := CopyOutput(&buf, strings.NewReader("hello\nworld\nEXIT 0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != "hello\nworld\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCopyOutputNonZeroExit(t *testing.T) {
	var buf bytes.Buffer
	err := CopyOutput(&buf, strings.NewReader("fail\nEXIT 7\n"))
	if err == nil {
		t.Fatal("expected ExitError")
	}
	e, ok := err.(*ExitError)
	if !ok || e.Code != 7 {
		t.Fatalf("got %v", err)
	}
	if got := buf.String(); got != "fail\n" {
		t.Fatalf("got %q", got)
	}
}

func TestOutputUDSPath(t *testing.T) {
	if got := OutputUDSPath("/jail/vsock.sock"); got != "/jail/vsock.sock_5253" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatExit(t *testing.T) {
	if got := FormatExit(3); got != "EXIT 3\n" {
		t.Fatalf("got %q", got)
	}
}
