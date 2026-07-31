package vsock

import (
	"bytes"
	"fmt"
	"io"
)

// ExitError is returned when the guest reports a non-zero EXIT trailer.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.Code)
}

// ExitCode returns the process exit code for err, or 0 if err is nil.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if e, ok := err.(*ExitError); ok {
		return e.Code
	}
	return 1
}

// CopyOutput streams r to w, stripping a trailing EXIT trailer. Non-zero
// EXIT codes become *ExitError. Incomplete trailing data (no newline) is
// written as-is unless it is an EXIT line.
func CopyOutput(w io.Writer, r io.Reader) error {
	s := &exitStripper{w: w}
	if _, err := io.Copy(s, r); err != nil {
		return err
	}
	if err := s.Flush(); err != nil {
		return err
	}
	if s.saw && s.code != 0 {
		return &ExitError{Code: s.code}
	}
	return nil
}

type exitStripper struct {
	w    io.Writer
	buf  []byte
	code int
	saw  bool
}

func (e *exitStripper) Write(p []byte) (int, error) {
	e.buf = append(e.buf, p...)
	for {
		i := bytes.IndexByte(e.buf, '\n')
		if i < 0 {
			break
		}
		line := e.buf[:i+1]
		e.buf = e.buf[i+1:]
		if code, ok := ParseExitLine(string(line)); ok {
			e.code = code
			e.saw = true
			continue
		}
		if _, err := e.w.Write(line); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (e *exitStripper) Flush() error {
	if len(e.buf) == 0 {
		return nil
	}
	if code, ok := ParseExitLine(string(e.buf)); ok {
		e.code = code
		e.saw = true
		e.buf = nil
		return nil
	}
	_, err := e.w.Write(e.buf)
	e.buf = nil
	return err
}
