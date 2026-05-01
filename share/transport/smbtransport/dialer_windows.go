//go:build windows

package smbtransport

import (
	"context"
	"errors"
	"net"
	"time"

	"golang.org/x/sys/windows"
)

func DialContext(ctx context.Context, address string, _ string, config Config) (net.Conn, error) {
	config = normalizeConfig(config)
	normalized, err := NormalizeDialAddress(address)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok && config.DialTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.DialTimeout)
		defer cancel()
	}

	for {
		handle, err := openPipe(normalized)
		if err == nil {
			return newConn(handle, normalized, config), nil
		}
		if !isDialRetryable(err) {
			return nil, err
		}

		timer := time.NewTimer(config.RetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func openPipe(address string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(address)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED,
		0,
	)
}

func isDialRetryable(err error) bool {
	return errors.Is(err, windows.ERROR_FILE_NOT_FOUND) ||
		errors.Is(err, windows.ERROR_PIPE_BUSY) ||
		errors.Is(err, windows.ERROR_NO_DATA)
}
