//go:build !windows && !linux && !darwin

package smbtransport

import (
	"context"
	"net"
)

func dialRemotePipeContext(_ context.Context, _ string, _ Config) (net.Conn, error) {
	return nil, ErrUnsupported
}
