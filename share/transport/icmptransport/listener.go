package icmptransport

import (
	"net"
)

type Listener struct {
	socket *packetSocket
	addr   Addr
}

func Listen(bind string) (*Listener, error) {
	return ListenConfig(bind, DefaultConfig())
}

func ListenConfig(bind string, config Config) (*Listener, error) {
	normalized, err := NormalizeListenAddress(bind)
	if err != nil {
		return nil, err
	}
	socket, err := newPacketSocket(normalized, config, true)
	if err != nil {
		return nil, err
	}
	return &Listener{
		socket: socket,
		addr:   Addr{IP: net.ParseIP(normalized)},
	}, nil
}

func (listener *Listener) Accept() (net.Conn, error) {
	conn, ok := <-listener.socket.acceptCh
	if !ok {
		return nil, net.ErrClosed
	}
	return conn, nil
}

func (listener *Listener) Close() error {
	return listener.socket.closeAccepting()
}

func (listener *Listener) Addr() net.Addr {
	return listener.addr
}
