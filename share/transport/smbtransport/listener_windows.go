//go:build windows

package smbtransport

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Listener struct {
	config Config
	addr   Addr

	mu       sync.Mutex
	handle   windows.Handle
	closed   bool
	closeErr error

	acceptHandle     windows.Handle
	acceptOverlapped *windows.Overlapped
}

var _ net.Listener = (*Listener)(nil)

func ListenConfig(ctx context.Context, address string, config Config) (net.Listener, error) {
	config = normalizeConfig(config)
	normalized, err := NormalizeListenAddress(address)
	if err != nil {
		return nil, err
	}
	handle, err := createPipe(normalized, true, config)
	if err != nil {
		return nil, err
	}
	listener := &Listener{
		config: config,
		addr:   Addr{Path: normalized},
		handle: handle,
	}
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				_ = listener.Close()
			}
		}()
	}
	return listener, nil
}

func (listener *Listener) Accept() (net.Conn, error) {
	for {
		conn, err := listener.acceptPipe()
		if errors.Is(err, windows.ERROR_NO_DATA) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
}

func (listener *Listener) acceptPipe() (net.Conn, error) {
	listener.mu.Lock()
	if listener.closed {
		listener.mu.Unlock()
		return nil, net.ErrClosed
	}

	handle := listener.handle
	if handle == 0 {
		var err error
		handle, err = createPipe(listener.addr.Path, false, listener.config)
		if err != nil {
			listener.mu.Unlock()
			return nil, err
		}
	} else {
		listener.handle = 0
	}

	overlapped, err := newOverlapped()
	if err != nil {
		_ = windows.CloseHandle(handle)
		listener.mu.Unlock()
		return nil, err
	}
	listener.acceptHandle = handle
	listener.acceptOverlapped = overlapped
	listener.mu.Unlock()

	connected := false
	err = windows.ConnectNamedPipe(handle, overlapped)
	switch {
	case err == nil, errors.Is(err, windows.ERROR_PIPE_CONNECTED):
		connected = true
	case isPending(err):
		_, err = waitForCompletion(handle, overlapped, timeZero(), net.ErrClosed)
		if err == nil {
			connected = true
		}
	}

	listener.mu.Lock()
	if listener.acceptHandle == handle {
		listener.acceptHandle = 0
		listener.acceptOverlapped = nil
	}
	closed := listener.closed
	listener.mu.Unlock()
	closeOverlapped(overlapped)

	if closed && !connected {
		_ = windows.CloseHandle(handle)
		return nil, net.ErrClosed
	}
	if !connected {
		_ = windows.CloseHandle(handle)
		if errors.Is(err, windows.ERROR_NO_DATA) {
			return nil, err
		}
		return nil, mapPipeError(err)
	}
	return newConn(handle, listener.addr.Path, listener.config), nil
}

func (listener *Listener) Close() error {
	listener.mu.Lock()
	defer listener.mu.Unlock()

	if listener.closed {
		return listener.closeErr
	}
	listener.closed = true

	if listener.handle != 0 {
		if err := disconnectPipe(listener.handle); err != nil && listener.closeErr == nil {
			listener.closeErr = err
		}
		if err := windows.CloseHandle(listener.handle); err != nil && listener.closeErr == nil {
			listener.closeErr = err
		}
		listener.handle = 0
	}
	if listener.acceptHandle != 0 && listener.acceptOverlapped != nil {
		if err := windows.CancelIoEx(listener.acceptHandle, listener.acceptOverlapped); err != nil &&
			!errors.Is(err, windows.ERROR_NOT_FOUND) &&
			listener.closeErr == nil {
			listener.closeErr = err
		}
	}
	return listener.closeErr
}

func (listener *Listener) Addr() net.Addr {
	return listener.addr
}

func createPipe(address string, first bool, config Config) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(address)
	if err != nil {
		return 0, err
	}
	mode := uint32(windows.PIPE_ACCESS_DUPLEX | windows.FILE_FLAG_OVERLAPPED)
	if first {
		mode |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	pipeMode := uint32(windows.PIPE_TYPE_BYTE | windows.PIPE_READMODE_BYTE | windows.PIPE_WAIT | windows.PIPE_ACCEPT_REMOTE_CLIENTS)

	security, err := securityAttributes(config.SecuritySDDL)
	if err != nil {
		return 0, err
	}
	return windows.CreateNamedPipe(
		name,
		mode,
		pipeMode,
		uint32(config.AcceptBacklog),
		uint32(config.BufferSize),
		uint32(config.BufferSize),
		0,
		security,
	)
}

func securityAttributes(sddl string) (*windows.SecurityAttributes, error) {
	if sddl == "" {
		return nil, nil
	}
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return nil, err
	}
	return &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: sd,
	}, nil
}

func disconnectPipe(handle windows.Handle) error {
	err := windows.DisconnectNamedPipe(handle)
	if errors.Is(err, windows.ERROR_PIPE_NOT_CONNECTED) ||
		errors.Is(err, windows.ERROR_NO_DATA) ||
		errors.Is(err, windows.ERROR_PIPE_LISTENING) {
		return nil
	}
	return err
}

func timeZero() time.Time {
	return time.Time{}
}
