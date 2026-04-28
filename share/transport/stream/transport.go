package stream

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"TengShe/share/transport/icmptransport"
	"TengShe/utils"
)

const (
	ProtocolTCP  = "tcp"
	ProtocolICMP = "icmp"

	defaultDialTimeout = 10 * time.Second
)

var ErrUnsupportedProtocol = errors.New("unsupported stream protocol")

type Transport interface {
	Protocol() string
	NormalizeListenAddress(address string) (string, error)
	NormalizeDialAddress(address string) (string, error)
	Listen(ctx context.Context, address string) (net.Listener, error)
	Dial(ctx context.Context, address string, bind string) (net.Conn, error)
	SupportsTLS() bool
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]Transport)
)

func init() {
	MustRegister(TCPTransport{})
	MustRegister(ICMPTransport{})
}

func Register(transport Transport) error {
	if transport == nil {
		return errors.New("nil stream transport")
	}
	protocol, err := NormalizeProtocol(transport.Protocol())
	if err != nil {
		return err
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[protocol]; exists {
		return fmt.Errorf("stream protocol %q already registered", protocol)
	}
	registry[protocol] = transport
	return nil
}

func MustRegister(transport Transport) {
	if err := Register(transport); err != nil {
		panic(err)
	}
}

func Get(protocol string) (Transport, error) {
	protocol, err := NormalizeProtocol(protocol)
	if err != nil {
		return nil, err
	}
	registryMu.RLock()
	transport := registry[protocol]
	registryMu.RUnlock()
	if transport == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProtocol, protocol)
	}
	return transport, nil
}

func NormalizeProtocol(protocol string) (string, error) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		return ProtocolTCP, nil
	}
	if strings.ContainsAny(protocol, " \t\r\n") {
		return "", fmt.Errorf("invalid stream protocol %q", protocol)
	}
	return protocol, nil
}

func NormalizeListenAddress(protocol, address string) (string, error) {
	transport, err := Get(protocol)
	if err != nil {
		return "", err
	}
	return transport.NormalizeListenAddress(address)
}

func NormalizeDialAddress(protocol, address string) (string, error) {
	transport, err := Get(protocol)
	if err != nil {
		return "", err
	}
	return transport.NormalizeDialAddress(address)
}

func Listen(ctx context.Context, protocol, address string) (net.Listener, error) {
	transport, err := Get(protocol)
	if err != nil {
		return nil, err
	}
	return transport.Listen(ctx, address)
}

func Dial(ctx context.Context, protocol, address, bind string) (net.Conn, error) {
	transport, err := Get(protocol)
	if err != nil {
		return nil, err
	}
	return transport.Dial(ctx, address, bind)
}

type TCPTransport struct{}

func (TCPTransport) Protocol() string { return ProtocolTCP }

func (TCPTransport) NormalizeListenAddress(address string) (string, error) {
	normalAddr, _, err := utils.CheckIPPort(address)
	return normalAddr, err
}

func (TCPTransport) NormalizeDialAddress(address string) (string, error) {
	normalAddr, _, err := utils.CheckIPPort(address)
	return normalAddr, err
}

func (TCPTransport) Listen(_ context.Context, address string) (net.Listener, error) {
	normalized, err := TCPTransport{}.NormalizeListenAddress(address)
	if err != nil {
		return nil, err
	}
	return net.Listen("tcp", normalized)
}

func (TCPTransport) Dial(ctx context.Context, address string, _ string) (net.Conn, error) {
	normalized, err := TCPTransport{}.NormalizeDialAddress(address)
	if err != nil {
		return nil, err
	}
	ctx, cancel := contextWithDefaultTimeout(ctx, defaultDialTimeout)
	defer cancel()
	return (&net.Dialer{}).DialContext(ctx, "tcp", normalized)
}

func (TCPTransport) SupportsTLS() bool { return true }

type ICMPTransport struct{}

func (ICMPTransport) Protocol() string { return ProtocolICMP }

func (ICMPTransport) NormalizeListenAddress(address string) (string, error) {
	return icmptransport.NormalizeListenAddress(address)
}

func (ICMPTransport) NormalizeDialAddress(address string) (string, error) {
	return icmptransport.NormalizePeerAddress(address)
}

func (ICMPTransport) Listen(_ context.Context, address string) (net.Listener, error) {
	return icmptransport.ListenConfig(address, icmptransport.DefaultConfigFromEnv())
}

func (ICMPTransport) Dial(ctx context.Context, address string, bind string) (net.Conn, error) {
	config := icmptransport.DefaultConfigFromEnv()
	ctx, cancel := contextWithDefaultTimeout(ctx, config.HandshakeTimeout)
	defer cancel()
	return icmptransport.DialContext(ctx, address, bind, config)
}

func (ICMPTransport) SupportsTLS() bool { return false }

func contextWithDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok || timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
