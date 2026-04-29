package dnstransport

import "net"

type Listener struct {
	socket *packetSocket
	addr   Addr
}

func Listen(address string) (*Listener, error) {
	return ListenConfig(address, DefaultConfig())
}

func ListenConfig(address string, config Config) (*Listener, error) {
	normalized, err := NormalizeListenAddress(address)
	if err != nil {
		return nil, err
	}
	socket, err := newListenSocket(normalized, config)
	if err != nil {
		return nil, err
	}
	return &Listener{
		socket: socket,
		addr:   Addr{Address: socket.pc.LocalAddr().String() + "/" + socket.domain},
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
