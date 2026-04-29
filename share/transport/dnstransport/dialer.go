package dnstransport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

type Dialer struct {
	Address string
	Bind    string
	Config  Config
}

func (dialer *Dialer) Dial() (net.Conn, error) {
	return DialConfig(dialer.Address, dialer.Bind, dialer.Config)
}

func Dial(address, bind string) (net.Conn, error) {
	return DialConfig(address, bind, DefaultConfig())
}

func DialConfig(address, bind string, config Config) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), normalizeConfig(config).HandshakeTimeout)
	defer cancel()
	return DialContext(ctx, address, bind, config)
}

func DialContext(ctx context.Context, address, bind string, config Config) (net.Conn, error) {
	socket, err := newDialSocket(address, bind, config)
	if err != nil {
		return nil, err
	}
	conn := newConn(socket, randomID(), socket.resolver, true)
	if err := socket.register(conn); err != nil {
		_ = socket.pc.Close()
		return nil, err
	}
	if err := conn.handshake(ctx); err != nil {
		conn.closeWithError(err)
		_ = socket.pc.Close()
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf(
				"DNS handshake timed out waiting for SYNACK from %s via domain %s: %w",
				udpAddrString(socket.resolver),
				socket.domain,
				err,
			)
		}
		return nil, err
	}
	return conn, nil
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
