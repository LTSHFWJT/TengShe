package websockettransport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	sharedtransport "TengShe/share/transport"

	"golang.org/x/net/websocket"
)

type Listener struct {
	config   Config
	addr     Addr
	path     string
	rawQuery string

	listener net.Listener
	server   *http.Server
	acceptCh chan net.Conn
	closed   chan struct{}

	closeOnce sync.Once
	activeMu  sync.Mutex
	active    map[*Conn]struct{}
}

func Listen(address string) (*Listener, error) {
	return ListenConfig(context.Background(), address, DefaultConfig())
}

func ListenConfig(ctx context.Context, address string, config Config) (*Listener, error) {
	config = normalizeConfig(config)
	parsed, err := parseURL(address, true, config)
	if err != nil {
		return nil, err
	}

	tcpListener, err := net.Listen("tcp", parsed.Host)
	if err != nil {
		return nil, err
	}
	parsed.Host = tcpListener.Addr().String()

	var netListener net.Listener = tcpListener
	if parsed.Scheme == "wss" {
		tlsConfig, err := sharedtransport.NewServerTLSConfig()
		if err != nil {
			_ = tcpListener.Close()
			return nil, err
		}
		netListener = tls.NewListener(tcpListener, tlsConfig)
	}

	listener := &Listener{
		config:   config,
		addr:     Addr{URL: parsed.String()},
		path:     parsed.Path,
		rawQuery: parsed.RawQuery,
		listener: netListener,
		acceptCh: make(chan net.Conn, config.AcceptBacklog),
		closed:   make(chan struct{}),
		active:   make(map[*Conn]struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc(parsed.Path, listener.handleWebSocket)
	listener.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: config.HandshakeTimeout,
	}

	go listener.serve()
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				_ = listener.Close()
			case <-listener.closed:
			}
		}()
	}

	return listener, nil
}

func (listener *Listener) Accept() (net.Conn, error) {
	select {
	case <-listener.closed:
		return nil, net.ErrClosed
	default:
	}

	select {
	case conn := <-listener.acceptCh:
		if wsConn, ok := conn.(*Conn); ok {
			listener.removeActive(wsConn)
		}
		select {
		case <-listener.closed:
			_ = conn.Close()
			return nil, net.ErrClosed
		default:
			return conn, nil
		}
	case <-listener.closed:
		return nil, net.ErrClosed
	}
}

func (listener *Listener) isClosed() bool {
	select {
	case <-listener.closed:
		return true
	default:
		return false
	}
}

func (listener *Listener) checkRequestPath(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	return req.URL.Path == listener.path && req.URL.RawQuery == listener.rawQuery
}

func (listener *Listener) waitForCloseOrDone(conn *Conn) {
	<-conn.done
}

func (listener *Listener) Close() error {
	var err error
	listener.closeOnce.Do(func() {
		close(listener.closed)
		listener.closeActive()
		if listener.server != nil {
			err = listener.server.Close()
		} else if listener.listener != nil {
			if closeErr := listener.listener.Close(); err == nil {
				err = closeErr
			}
		}
	})
	return err
}

func (listener *Listener) Addr() net.Addr {
	return listener.addr
}

func (listener *Listener) serve() {
	err := listener.server.Serve(listener.listener)
	if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		_ = listener.Close()
	}
}

func (listener *Listener) handleWebSocket(w http.ResponseWriter, req *http.Request) {
	server := websocket.Server{
		Handshake: listener.handshake,
		Handler:   listener.acceptWebSocket,
	}
	server.ServeHTTP(w, req)
}

func (listener *Listener) handshake(config *websocket.Config, req *http.Request) error {
	if !listener.checkRequestPath(req) {
		return http.ErrNotSupported
	}
	if !offeredSubprotocol(config.Protocol, Subprotocol) {
		return fmt.Errorf("missing WebSocket subprotocol %s", Subprotocol)
	}
	config.Protocol = []string{Subprotocol}
	if config.Header == nil {
		config.Header = make(http.Header)
	}
	config.Header.Set("X-TengShe-Transport", TransportName)
	return nil
}

func (listener *Listener) acceptWebSocket(ws *websocket.Conn) {
	conn := newConn(ws, listener.config, listener.removeActive)
	if listener.isClosed() {
		_ = conn.Close()
		return
	}
	listener.addActive(conn)

	select {
	case listener.acceptCh <- conn:
		listener.removeActive(conn)
		listener.waitForCloseOrDone(conn)
	case <-listener.closed:
		_ = conn.Close()
	}
}

func (listener *Listener) addActive(conn *Conn) {
	listener.activeMu.Lock()
	listener.active[conn] = struct{}{}
	listener.activeMu.Unlock()
}

func (listener *Listener) removeActive(conn *Conn) {
	listener.activeMu.Lock()
	delete(listener.active, conn)
	listener.activeMu.Unlock()
}

func (listener *Listener) closeActive() {
	listener.activeMu.Lock()
	active := make([]*Conn, 0, len(listener.active))
	for conn := range listener.active {
		active = append(active, conn)
	}
	listener.activeMu.Unlock()

	for _, conn := range active {
		_ = conn.Close()
	}
}

func offeredSubprotocol(offered []string, expected string) bool {
	for _, protocol := range offered {
		if strings.TrimSpace(protocol) == expected {
			return true
		}
	}
	return false
}

var _ net.Listener = (*Listener)(nil)
