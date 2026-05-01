//go:build windows

package smbtransport

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

type Conn struct {
	handle windows.Handle
	addr   Addr

	ioTimeout time.Duration
	maxChunk  int

	readMu  sync.Mutex
	writeMu sync.Mutex

	deadlineMu    sync.Mutex
	readDeadline  time.Time
	writeDeadline time.Time

	closeOnce sync.Once
	closeErr  error
}

var _ net.Conn = (*Conn)(nil)

func newConn(handle windows.Handle, path string, config Config) *Conn {
	config = normalizeConfig(config)
	return &Conn{
		handle:    handle,
		addr:      Addr{Path: path},
		ioTimeout: config.IOTimeout,
		maxChunk:  config.MaxChunk,
	}
}

func (conn *Conn) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	conn.readMu.Lock()
	defer conn.readMu.Unlock()

	overlapped, err := newOverlapped()
	if err != nil {
		return 0, err
	}
	defer closeOverlapped(overlapped)

	var done uint32
	err = windows.ReadFile(conn.handle, b, &done, overlapped)
	n, err := conn.completeRequest("read", done, err, conn.currentReadDeadline(), overlapped)
	return n, mapPipeError(err)
}

func (conn *Conn) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	conn.writeMu.Lock()
	defer conn.writeMu.Unlock()

	total := 0
	for total < len(b) {
		end := total + conn.maxChunk
		if end > len(b) {
			end = len(b)
		}
		n, err := conn.writeChunk(b[total:end])
		total += n
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, io.ErrUnexpectedEOF
		}
	}
	return total, nil
}

func (conn *Conn) writeChunk(b []byte) (int, error) {
	overlapped, err := newOverlapped()
	if err != nil {
		return 0, err
	}
	defer closeOverlapped(overlapped)

	var done uint32
	err = windows.WriteFile(conn.handle, b, &done, overlapped)
	n, err := conn.completeRequest("write", done, err, conn.currentWriteDeadline(), overlapped)
	return n, mapPipeError(err)
}

func (conn *Conn) completeRequest(op string, done uint32, err error, deadline time.Time, overlapped *windows.Overlapped) (int, error) {
	if err == nil {
		return int(done), nil
	}
	if !isPending(err) {
		return int(done), err
	}

	done, err = waitForCompletion(conn.handle, overlapped, deadline, timeoutError{op: op, addr: conn.addr.String()})
	return int(done), err
}

func (conn *Conn) Close() error {
	conn.closeOnce.Do(func() {
		conn.closeErr = windows.CloseHandle(conn.handle)
	})
	return conn.closeErr
}

func (conn *Conn) LocalAddr() net.Addr {
	return conn.addr
}

func (conn *Conn) RemoteAddr() net.Addr {
	return conn.addr
}

func (conn *Conn) SetDeadline(t time.Time) error {
	conn.deadlineMu.Lock()
	conn.readDeadline = t
	conn.writeDeadline = t
	conn.deadlineMu.Unlock()
	return nil
}

func (conn *Conn) SetReadDeadline(t time.Time) error {
	conn.deadlineMu.Lock()
	conn.readDeadline = t
	conn.deadlineMu.Unlock()
	return nil
}

func (conn *Conn) SetWriteDeadline(t time.Time) error {
	conn.deadlineMu.Lock()
	conn.writeDeadline = t
	conn.deadlineMu.Unlock()
	return nil
}

func (conn *Conn) currentReadDeadline() time.Time {
	conn.deadlineMu.Lock()
	defer conn.deadlineMu.Unlock()
	if !conn.readDeadline.IsZero() {
		return conn.readDeadline
	}
	if conn.ioTimeout > 0 {
		return time.Now().Add(conn.ioTimeout)
	}
	return time.Time{}
}

func (conn *Conn) currentWriteDeadline() time.Time {
	conn.deadlineMu.Lock()
	defer conn.deadlineMu.Unlock()
	if !conn.writeDeadline.IsZero() {
		return conn.writeDeadline
	}
	if conn.ioTimeout > 0 {
		return time.Now().Add(conn.ioTimeout)
	}
	return time.Time{}
}

func newOverlapped() (*windows.Overlapped, error) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	return &windows.Overlapped{HEvent: event}, nil
}

func closeOverlapped(overlapped *windows.Overlapped) {
	if overlapped != nil && overlapped.HEvent != 0 {
		_ = windows.CloseHandle(overlapped.HEvent)
		overlapped.HEvent = 0
	}
}

func waitForCompletion(handle windows.Handle, overlapped *windows.Overlapped, deadline time.Time, timeoutErr error) (uint32, error) {
	waitMilliseconds := uint32(windows.INFINITE)
	if !deadline.IsZero() {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			_ = windows.CancelIoEx(handle, overlapped)
			var discarded uint32
			_ = windows.GetOverlappedResult(handle, overlapped, &discarded, true)
			return 0, timeoutErr
		}
		waitMilliseconds = uint32(remaining / time.Millisecond)
		if waitMilliseconds == 0 {
			waitMilliseconds = 1
		}
	}

	event, err := windows.WaitForSingleObject(overlapped.HEvent, waitMilliseconds)
	if err != nil {
		return 0, err
	}
	if event == uint32(windows.WAIT_TIMEOUT) {
		_ = windows.CancelIoEx(handle, overlapped)
		var discarded uint32
		_ = windows.GetOverlappedResult(handle, overlapped, &discarded, true)
		return 0, timeoutErr
	}
	if event != uint32(windows.WAIT_OBJECT_0) {
		return 0, fmt.Errorf("unexpected wait result %d", event)
	}

	var done uint32
	err = windows.GetOverlappedResult(handle, overlapped, &done, false)
	return done, err
}

func isPending(err error) bool {
	return errors.Is(err, windows.ERROR_IO_PENDING) || errors.Is(err, windows.ERROR_IO_INCOMPLETE)
}

func mapPipeError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, windows.ERROR_BROKEN_PIPE),
		errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED),
		errors.Is(err, windows.ERROR_NO_DATA):
		return io.EOF
	case errors.Is(err, windows.ERROR_OPERATION_ABORTED):
		return net.ErrClosed
	default:
		return err
	}
}
