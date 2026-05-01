package websockettransport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	sharedtransport "TengShe/share/transport"

	"golang.org/x/net/websocket"
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
	config = normalizeConfig(config)
	ctx, cancel := context.WithTimeout(context.Background(), config.HandshakeTimeout)
	defer cancel()
	return DialContext(ctx, address, bind, config)
}

func DialContext(ctx context.Context, address, bind string, config Config) (net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	config = normalizeConfig(config)
	parsed, err := parseURL(address, false, config)
	if err != nil {
		return nil, err
	}

	requestURL := *parsed
	if config.Host != "" {
		if err := validateHostOverride(config.Host); err != nil {
			return nil, err
		}
		requestURL.Host = config.Host
	}
	wsConfig, err := websocket.NewConfig(requestURL.String(), config.Origin)
	if err != nil {
		return nil, err
	}
	wsConfig.Protocol = []string{Subprotocol}
	wsConfig.Header = cloneHeader(config.Headers)
	dialer := &net.Dialer{}
	if bind != "" {
		localAddr, err := net.ResolveTCPAddr("tcp", bind)
		if err != nil {
			return nil, err
		}
		dialer.LocalAddr = localAddr
	}

	conn, err := dialer.DialContext(ctx, "tcp", parsed.Host)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()

	if parsed.Scheme == "wss" {
		tlsConfig, err := sharedtransport.NewClientTLSConfig(serverNameForTLS(&requestURL, parsed.Hostname()))
		if err != nil {
			return nil, err
		}
		wsConfig.TlsConfig = tlsConfig
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return nil, err
		}
		conn = tlsConn
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
		defer conn.SetDeadline(time.Time{})
	}

	ws, err := websocket.NewClient(wsConfig, conn)
	if err != nil {
		return nil, err
	}
	success = true
	return newConn(ws, config, nil), nil
}

func cloneHeader(headers http.Header) http.Header {
	cloned := make(http.Header)
	for key, values := range headers {
		key = http.CanonicalHeaderKey(strings.TrimSpace(key))
		if key == "" || isReservedWebSocketHeader(key) {
			continue
		}
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" || !isSafeHeaderValue(value) {
				continue
			}
			cloned.Add(key, value)
		}
	}
	return cloned
}

func validateHostOverride(value string) error {
	if value == "" {
		return nil
	}
	if strings.Contains(value, "://") || strings.ContainsAny(value, " \t\r\n/\\") {
		return fmt.Errorf("invalid WebSocket host override %q", value)
	}
	return nil
}

func serverNameForTLS(location *url.URL, fallback string) string {
	if location == nil {
		return fallback
	}
	host := location.Hostname()
	if host == "" {
		return fallback
	}
	return host
}
