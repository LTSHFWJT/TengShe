package websockettransport

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type Addr struct {
	URL string
}

func (addr Addr) Network() string {
	return TransportName
}

func (addr Addr) String() string {
	if addr.URL == "" {
		return "ws://"
	}
	return addr.URL
}

func NormalizeListenAddress(value string) (string, error) {
	return normalizeAddress(value, true, DefaultConfigFromEnv().Path)
}

func NormalizeDialAddress(value string) (string, error) {
	return normalizeAddress(value, false, DefaultConfigFromEnv().Path)
}

func normalizeListenAddressWithConfig(value string, config Config) (string, error) {
	return normalizeAddress(value, true, normalizeConfig(config).Path)
}

func normalizeDialAddressWithConfig(value string, config Config) (string, error) {
	return normalizeAddress(value, false, normalizeConfig(config).Path)
}

func normalizeAddress(value string, listen bool, defaultPath string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if listen {
			return "", errors.New("empty WebSocket listen address")
		}
		return "", errors.New("empty WebSocket dial address")
	}

	if hasWebSocketScheme(value) {
		parsed, err := url.Parse(value)
		if err != nil {
			return "", err
		}
		return normalizeURL(parsed, listen, defaultPath)
	}
	if strings.Contains(value, "://") {
		return "", fmt.Errorf("unsupported WebSocket scheme in address %q", value)
	}

	hostPort, path, _ := strings.Cut(value, "/")
	host, port, err := splitHostPort(hostPort, listen)
	if err != nil {
		return "", err
	}
	if listen && host == "" {
		host = "0.0.0.0"
	}
	if !listen && host == "" {
		return "", fmt.Errorf("invalid WebSocket dial address %q: empty host", value)
	}

	normalized := &url.URL{
		Scheme: "ws",
		Host:   net.JoinHostPort(host, port),
		Path:   normalizeAddressPath(path, defaultPath),
	}
	return normalized.String(), nil
}

func normalizeURL(parsed *url.URL, listen bool, defaultPath string) (string, error) {
	if parsed == nil {
		return "", errors.New("nil WebSocket URL")
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return "", fmt.Errorf("unsupported WebSocket scheme %q", parsed.Scheme)
	}
	if parsed.User != nil {
		return "", errors.New("WebSocket address must not include user info")
	}

	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		return "", fmt.Errorf("WebSocket address %q must include a port", parsed.String())
	}
	if err := validatePort(port, listen); err != nil {
		return "", err
	}
	if listen && host == "" {
		host = "0.0.0.0"
	}
	if !listen && host == "" {
		return "", fmt.Errorf("invalid WebSocket dial address %q: empty host", parsed.String())
	}

	normalized := &url.URL{
		Scheme:   parsed.Scheme,
		Host:     net.JoinHostPort(host, port),
		Path:     normalizeAddressPath(parsed.Path, defaultPath),
		RawQuery: parsed.RawQuery,
	}
	return normalized.String(), nil
}

func parseURL(value string, listen bool, config Config) (*url.URL, error) {
	var normalized string
	var err error
	if listen {
		normalized, err = normalizeListenAddressWithConfig(value, config)
	} else {
		normalized, err = normalizeDialAddressWithConfig(value, config)
	}
	if err != nil {
		return nil, err
	}
	return url.Parse(normalized)
}

func hasWebSocketScheme(value string) bool {
	value = strings.ToLower(value)
	return strings.HasPrefix(value, "ws://") || strings.HasPrefix(value, "wss://")
}

func splitHostPort(value string, listen bool) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", errors.New("empty WebSocket host:port")
	}
	if listen && isDecimal(value) {
		if err := validatePort(value, true); err != nil {
			return "", "", err
		}
		return "0.0.0.0", value, nil
	}
	if host, port, err := net.SplitHostPort(value); err == nil {
		if err := validatePort(port, listen); err != nil {
			return "", "", err
		}
		return host, port, nil
	}
	if strings.Count(value, ":") == 1 {
		host, port, _ := strings.Cut(value, ":")
		if err := validatePort(port, listen); err != nil {
			return "", "", err
		}
		return host, port, nil
	}
	return "", "", fmt.Errorf("invalid WebSocket address %q: expected host:port/path", value)
}

func normalizeAddressPath(path string, defaultPath string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return normalizePath(defaultPath)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func validatePort(value string, allowZero bool) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < 0 || port > 65535 {
		return fmt.Errorf("invalid WebSocket port %q", value)
	}
	if port == 0 && !allowZero {
		return fmt.Errorf("invalid WebSocket port %q: zero is only valid for listening", value)
	}
	return nil
}

func isDecimal(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
