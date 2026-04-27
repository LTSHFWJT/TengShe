package icmptransport

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type Addr struct {
	IP net.IP
}

func (addr Addr) Network() string {
	return TransportName
}

func (addr Addr) String() string {
	if addr.IP == nil {
		return "icmp://0.0.0.0"
	}
	return "icmp://" + addr.IP.String()
}

func NormalizeListenAddress(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0.0.0.0", nil
	}
	if isDecimal(value) {
		return "0.0.0.0", nil
	}
	ip := net.ParseIP(value)
	if ip == nil {
		return "", fmt.Errorf("invalid ICMP listen address %q", value)
	}
	if ip.To4() == nil {
		return "", fmt.Errorf("ICMP transport currently supports IPv4 only: %q", value)
	}
	return ip.String(), nil
}

func NormalizePeerAddress(value string) (string, error) {
	host, err := peerHost(value)
	if err != nil {
		return "", err
	}
	ip, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return "", err
	}
	if ip == nil || ip.IP == nil || ip.IP.To4() == nil {
		return "", fmt.Errorf("ICMP transport currently supports IPv4 only: %q", value)
	}
	return ip.IP.String(), nil
}

func peerHost(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("empty ICMP peer address")
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		if host == "" {
			return "", fmt.Errorf("invalid ICMP peer address %q", value)
		}
		return host, nil
	}
	if strings.Count(value, ":") == 1 {
		host, port, ok := strings.Cut(value, ":")
		if ok && isPort(port) {
			return host, nil
		}
	}
	return value, nil
}

func isPort(value string) bool {
	if !isDecimal(value) {
		return false
	}
	port, err := strconv.Atoi(value)
	return err == nil && port >= 0 && port <= 65535
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
