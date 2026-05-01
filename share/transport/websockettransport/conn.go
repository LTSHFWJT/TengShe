package websockettransport

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/net/websocket"
)

type Conn struct {
	*websocket.Conn
	localAddr       net.Addr
	remoteAddr      net.Addr
	done            chan struct{}
	doneOnce        sync.Once
	readMu          sync.Mutex
	readBuf         []byte
	writeMu         sync.Mutex
	maxFramePayload int
	onDone          func(*Conn)
}

var errNonBinaryFrame = errors.New("websocket transport received non-binary data frame")

var binaryFrameCodec = websocket.Codec{
	Marshal: func(v interface{}) ([]byte, byte, error) {
		data, ok := v.([]byte)
		if !ok {
			return nil, websocket.UnknownFrame, websocket.ErrNotSupported
		}
		return data, websocket.BinaryFrame, nil
	},
	Unmarshal: func(data []byte, payloadType byte, v interface{}) error {
		if payloadType != websocket.BinaryFrame {
			return errNonBinaryFrame
		}
		out, ok := v.(*[]byte)
		if !ok {
			return websocket.ErrNotSupported
		}
		*out = data
		return nil
	},
}

func newConn(ws *websocket.Conn, config Config, onDone func(*Conn)) *Conn {
	config = normalizeConfig(config)
	ws.PayloadType = websocket.BinaryFrame
	ws.MaxPayloadBytes = config.MaxFramePayload
	localAddr, remoteAddr := websocketAddrs(ws)
	return &Conn{
		Conn:            ws,
		localAddr:       localAddr,
		remoteAddr:      remoteAddr,
		done:            make(chan struct{}),
		maxFramePayload: config.MaxFramePayload,
		onDone:          onDone,
	}
}

func (conn *Conn) LocalAddr() net.Addr {
	if conn.localAddr != nil {
		return conn.localAddr
	}
	return Addr{URL: "ws://local"}
}

func (conn *Conn) RemoteAddr() net.Addr {
	if conn.remoteAddr != nil {
		return conn.remoteAddr
	}
	return Addr{URL: "ws://remote"}
}

func (conn *Conn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	conn.readMu.Lock()
	defer conn.readMu.Unlock()

	for len(conn.readBuf) == 0 {
		var frame []byte
		if err := binaryFrameCodec.Receive(conn.Conn, &frame); err != nil {
			conn.closeAfterError(err)
			return 0, err
		}
		conn.readBuf = frame
	}

	n := copy(p, conn.readBuf)
	conn.readBuf = conn.readBuf[n:]
	return n, nil
}

func (conn *Conn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	conn.writeMu.Lock()
	defer conn.writeMu.Unlock()
	conn.Conn.PayloadType = websocket.BinaryFrame

	limit := conn.maxFramePayload
	if limit <= 0 {
		limit = defaultMaxFramePayload
	}

	written := 0
	for written < len(p) {
		end := written + limit
		if end > len(p) {
			end = len(p)
		}
		chunk := p[written:end]
		for len(chunk) > 0 {
			n, err := conn.Conn.Write(chunk)
			if n > 0 {
				written += n
				chunk = chunk[n:]
			}
			if shouldMarkDone(err) {
				conn.closeAfterError(err)
			}
			if err != nil {
				return written, err
			}
			if n == 0 {
				return written, io.ErrShortWrite
			}
		}
	}
	return written, nil
}

func (conn *Conn) Close() error {
	err := conn.Conn.Close()
	conn.markDone()
	return err
}

func (conn *Conn) closeAfterError(err error) {
	if !shouldMarkDone(err) {
		return
	}
	_ = conn.Conn.Close()
	conn.markDone()
}

func (conn *Conn) markDone() {
	conn.doneOnce.Do(func() {
		close(conn.done)
		if conn.onDone != nil {
			conn.onDone(conn)
		}
	})
}

func shouldMarkDone(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return false
	}
	return true
}

func websocketAddrs(ws *websocket.Conn) (net.Addr, net.Addr) {
	if ws == nil {
		return Addr{URL: "ws://local"}, Addr{URL: "ws://remote"}
	}

	config := ws.Config()
	if req := ws.Request(); req != nil {
		local := ""
		if config != nil {
			local = urlString(config.Location)
		}
		if local == "" {
			local = requestURLString(req)
		}

		remote := strings.TrimSpace(req.RemoteAddr)
		if remote == "" {
			remote = "ws://remote"
		}
		return Addr{URL: local}, Addr{URL: remote}
	}

	local := ""
	remote := ""
	if config != nil {
		local = urlString(config.Origin)
		remote = urlString(config.Location)
	}
	if local == "" {
		local = "ws://local"
	}
	if remote == "" {
		remote = "ws://remote"
	}
	return Addr{URL: local}, Addr{URL: remote}
}

func urlString(value *url.URL) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func requestURLString(req *http.Request) string {
	scheme := "ws"
	if req.TLS != nil {
		scheme = "wss"
	}

	host := strings.TrimSpace(req.Host)
	if host == "" {
		host = "localhost"
	}

	path := "/"
	if req.URL != nil {
		path = req.URL.RequestURI()
		if path == "" {
			path = "/"
		}
	}
	return scheme + "://" + host + path
}

var _ net.Conn = (*Conn)(nil)
