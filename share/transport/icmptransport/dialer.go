package icmptransport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

type Dialer struct {
	Peer   string
	Bind   string
	Config Config
}

func (dialer *Dialer) Dial() (net.Conn, error) {
	return DialConfig(dialer.Peer, dialer.Bind, dialer.Config)
}

func Dial(peer, bind string) (net.Conn, error) {
	return DialConfig(peer, bind, DefaultConfig())
}

func DialConfig(peer, bind string, config Config) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), normalizeConfig(config).HandshakeTimeout)
	defer cancel()
	return DialContext(ctx, peer, bind, config)
}

func DialContext(ctx context.Context, peer, bind string, config Config) (net.Conn, error) {
	peerAddr, err := resolvePeer(peer)
	if err != nil {
		return nil, err
	}
	socket, err := newPacketSocket(bind, config, false)
	if err != nil {
		return nil, err
	}
	conn := newConn(socket, randomID(), peerAddr, true)
	if err := socket.register(conn); err != nil {
		_ = socket.pc.Close()
		return nil, err
	}
	if err := conn.handshake(ctx); err != nil {
		conn.closeWithError(err)
		_ = socket.pc.Close()
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf(
				"ICMP handshake timed out waiting for SYNACK from %s; "+
					"ensure the peer is running in ICMP passive mode with raw socket permission and ICMP echo traffic is allowed: %w",
				peerAddr.IP.String(),
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
