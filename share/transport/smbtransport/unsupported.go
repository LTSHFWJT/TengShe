//go:build !windows

package smbtransport

import (
	"context"
	"errors"
	"net"
)

var ErrUnsupported = errors.New("smb named pipe listener is only supported on windows; linux/darwin can dial remote pipe://host/name")

func ListenConfig(ctx context.Context, address string, config Config) (net.Listener, error) {
	if _, err := NormalizeListenAddress(address); err != nil {
		return nil, err
	}
	return nil, ErrUnsupported
}

func DialContext(ctx context.Context, address string, _ string, config Config) (net.Conn, error) {
	config = normalizeConfig(config)
	normalized, err := NormalizeDialAddress(address)
	if err != nil {
		return nil, err
	}
	host, _, err := splitNativePipePath(normalized)
	if err != nil {
		return nil, err
	}
	if isLocalPipeHost(host) {
		return nil, ErrUnsupported
	}
	return dialRemotePipeContext(ctx, normalized, config)
}
