//go:build linux || darwin

package smbtransport

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	jsmb "github.com/jfjallid/go-smb/smb"
	"github.com/jfjallid/go-smb/spnego"
)

type remotePipeConn struct {
	session *jsmb.Connection
	file    *jsmb.File
	rawConn net.Conn
	addr    Addr
	config  Config

	readMu  sync.Mutex
	writeMu sync.Mutex

	deadlineMu    sync.Mutex
	readDeadline  time.Time
	writeDeadline time.Time

	closeOnce sync.Once
	closeErr  error
	closed    chan struct{}
}

var _ net.Conn = (*remotePipeConn)(nil)

func dialRemotePipeContext(ctx context.Context, address string, config Config) (net.Conn, error) {
	config = normalizeConfig(config)
	host, pipeName, err := splitNativePipePath(address)
	if err != nil {
		return nil, err
	}
	if isLocalPipeHost(host) {
		return nil, ErrUnsupported
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
		conn, err := openRemotePipeOnce(host, pipeName, address, config)
		if err == nil {
			return conn, nil
		}
		if !isRemotePipeRetryable(err) {
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

func openRemotePipeOnce(host, pipeName, address string, config Config) (*remotePipeConn, error) {
	dialer := &smbClientDialer{timeout: config.DialTimeout}
	session, err := jsmb.NewConnection(jsmb.Options{
		Host:        host,
		Port:        config.SMBPort,
		DialTimeout: config.DialTimeout,
		ProxyDialer: dialer,
		Initiator: &spnego.NTLMInitiator{
			User:        config.SMBUser,
			Password:    config.SMBPassword,
			Domain:      config.SMBDomain,
			LocalUser:   config.SMBLocalUser,
			NullSession: config.SMBNullSession,
			Workstation: config.SMBWorkstation,
		},
	})
	if err != nil {
		return nil, mapRemotePipeError(err)
	}

	cleanup := true
	defer func() {
		if cleanup {
			session.Close()
		}
	}()

	if err := session.TreeConnect("IPC$"); err != nil {
		return nil, mapRemotePipeError(err)
	}

	opts := jsmb.NewCreateReqOpts()
	opts.DesiredAccess = jsmb.FAccMaskFileReadData |
		jsmb.FAccMaskFileWriteData |
		jsmb.FAccMaskFileReadAttributes |
		jsmb.FAccMaskFileWriteAttributes |
		jsmb.FAccMaskReadControl |
		jsmb.FAccMaskSynchronize
	opts.ShareAccess = jsmb.FileShareRead | jsmb.FileShareWrite
	opts.CreateDisp = jsmb.FileOpen
	opts.CreateOpts = 0

	file, err := session.OpenFileExt("IPC$", pipeName, opts)
	if err != nil {
		return nil, mapRemotePipeError(err)
	}

	cleanup = false
	return &remotePipeConn{
		session: session,
		file:    file,
		rawConn: dialer.Conn(),
		addr:    Addr{Path: address},
		config:  config,
		closed:  make(chan struct{}),
	}, nil
}

func (conn *remotePipeConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	conn.readMu.Lock()
	defer conn.readMu.Unlock()

	select {
	case <-conn.closed:
		return 0, net.ErrClosed
	default:
	}

	n, err := conn.runWithDeadline("read", conn.currentReadDeadline(), func() (int, error) {
		return conn.file.ReadFile(p, 0)
	})
	return n, mapRemotePipeError(err)
}

func (conn *remotePipeConn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	conn.writeMu.Lock()
	defer conn.writeMu.Unlock()

	select {
	case <-conn.closed:
		return 0, net.ErrClosed
	default:
	}

	total := 0
	for total < len(p) {
		end := total + conn.config.MaxChunk
		if end > len(p) {
			end = len(p)
		}
		n, err := conn.runWithDeadline("write", conn.currentWriteDeadline(), func() (int, error) {
			return conn.file.WriteFile(p[total:end], 0)
		})
		total += n
		if err != nil {
			return total, mapRemotePipeError(err)
		}
		if n == 0 {
			return total, io.ErrUnexpectedEOF
		}
	}
	return total, nil
}

func (conn *remotePipeConn) runWithDeadline(op string, deadline time.Time, fn func() (int, error)) (int, error) {
	if deadline.IsZero() && conn.config.IOTimeout > 0 {
		deadline = time.Now().Add(conn.config.IOTimeout)
	}
	if deadline.IsZero() {
		return fn()
	}
	if time.Now().After(deadline) {
		_ = conn.Close()
		return 0, timeoutError{op: op, addr: conn.addr.String()}
	}

	type result struct {
		n   int
		err error
	}
	done := make(chan result, 1)
	go func() {
		n, err := fn()
		done <- result{n: n, err: err}
	}()

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()

	select {
	case res := <-done:
		return res.n, res.err
	case <-timer.C:
		_ = conn.Close()
		return 0, timeoutError{op: op, addr: conn.addr.String()}
	case <-conn.closed:
		return 0, net.ErrClosed
	}
}

func (conn *remotePipeConn) Close() error {
	conn.closeOnce.Do(func() {
		close(conn.closed)
		if conn.rawConn != nil {
			conn.closeErr = conn.rawConn.Close()
		} else if conn.file != nil {
			conn.closeErr = conn.file.CloseFile()
		}
		if conn.session != nil {
			_ = conn.session.TreeDisconnect("IPC$")
			conn.session.Close()
		}
	})
	return conn.closeErr
}

func (conn *remotePipeConn) LocalAddr() net.Addr {
	return conn.addr
}

func (conn *remotePipeConn) RemoteAddr() net.Addr {
	return conn.addr
}

func (conn *remotePipeConn) SetDeadline(t time.Time) error {
	conn.deadlineMu.Lock()
	conn.readDeadline = t
	conn.writeDeadline = t
	conn.deadlineMu.Unlock()
	return nil
}

func (conn *remotePipeConn) SetReadDeadline(t time.Time) error {
	conn.deadlineMu.Lock()
	conn.readDeadline = t
	conn.deadlineMu.Unlock()
	return nil
}

func (conn *remotePipeConn) SetWriteDeadline(t time.Time) error {
	conn.deadlineMu.Lock()
	conn.writeDeadline = t
	conn.deadlineMu.Unlock()
	return nil
}

func (conn *remotePipeConn) currentReadDeadline() time.Time {
	conn.deadlineMu.Lock()
	defer conn.deadlineMu.Unlock()
	return conn.readDeadline
}

func (conn *remotePipeConn) currentWriteDeadline() time.Time {
	conn.deadlineMu.Lock()
	defer conn.deadlineMu.Unlock()
	return conn.writeDeadline
}

func isRemotePipeRetryable(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "pipe not available") ||
		strings.Contains(message, "pipe busy") ||
		strings.Contains(message, "requested file does not exist") ||
		strings.Contains(message, "path to the specified directory was not found")
}

func mapRemotePipeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return err
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "pipe_disconnected") ||
		strings.Contains(message, "pipe_broken") ||
		strings.Contains(message, "remote connection has closed") ||
		strings.Contains(message, "can't operate on a closed file") {
		return io.EOF
	}
	return err
}

type smbClientDialer struct {
	timeout time.Duration

	mu   sync.Mutex
	conn net.Conn
}

func (dialer *smbClientDialer) Dial(network, address string) (net.Conn, error) {
	ctx := context.Background()
	var cancel context.CancelFunc
	if dialer.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, dialer.timeout)
		defer cancel()
	}
	return dialer.DialContext(ctx, network, address)
}

func (dialer *smbClientDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	dialer.mu.Lock()
	dialer.conn = conn
	dialer.mu.Unlock()
	return conn, nil
}

func (dialer *smbClientDialer) Conn() net.Conn {
	dialer.mu.Lock()
	defer dialer.mu.Unlock()
	return dialer.conn
}
