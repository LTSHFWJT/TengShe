package dnstransport

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type Addr struct {
	Address string
}

func (addr Addr) Network() string {
	return TransportName
}

func (addr Addr) String() string {
	if addr.Address == "" {
		return "dns://"
	}
	return "dns://" + addr.Address
}

type ListenSpec struct {
	Bind   string
	Domain string
}

type DialSpec struct {
	Domain   string
	Resolver string
}

func NormalizeListenAddress(value string) (string, error) {
	spec, err := parseListenSpec(value)
	if err != nil {
		return "", err
	}
	return spec.String(), nil
}

func NormalizeDialAddress(value string) (string, error) {
	spec, err := parseDialSpec(value)
	if err != nil {
		return "", err
	}
	return spec.String(), nil
}

func (spec ListenSpec) String() string {
	return spec.Bind + "/" + spec.Domain
}

func (spec DialSpec) String() string {
	if spec.Resolver == "" {
		return spec.Domain
	}
	return spec.Domain + "@" + spec.Resolver
}

func parseListenSpec(value string) (ListenSpec, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "dns://")
	value = strings.TrimPrefix(value, "udp://")
	if value == "" {
		return ListenSpec{}, errors.New("DNS listen address requires /domain, for example 0.0.0.0:5353/t.example")
	}
	bind, domain, ok := strings.Cut(value, "/")
	if !ok {
		return ListenSpec{}, fmt.Errorf("invalid DNS listen address %q: expected host:port/domain", value)
	}
	bind, err := normalizeBind(bind)
	if err != nil {
		return ListenSpec{}, err
	}
	domain, err = normalizeDomain(domain)
	if err != nil {
		return ListenSpec{}, err
	}
	return ListenSpec{Bind: bind, Domain: domain}, nil
}

func parseDialSpec(value string) (DialSpec, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "dns://")
	if value == "" {
		return DialSpec{}, errors.New("empty DNS dial address")
	}
	domain, resolver, _ := strings.Cut(value, "@")
	domain, err := normalizeDomain(domain)
	if err != nil {
		return DialSpec{}, err
	}
	if resolver != "" {
		resolver, err = normalizeResolver(resolver)
		if err != nil {
			return DialSpec{}, err
		}
	} else {
		resolver = defaultResolverAddress()
	}
	return DialSpec{Domain: domain, Resolver: resolver}, nil
}

func normalizeBind(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "0.0.0.0:53"
	}
	if isDecimal(value) {
		value = "0.0.0.0:" + value
	}
	if host, port, err := net.SplitHostPort(value); err == nil {
		if host == "" {
			host = "0.0.0.0"
		}
		if err := validatePort(port); err != nil {
			return "", err
		}
		return net.JoinHostPort(host, port), nil
	}
	if strings.Count(value, ":") == 0 {
		return net.JoinHostPort(value, "53"), nil
	}
	return "", fmt.Errorf("invalid DNS bind address %q", value)
}

func normalizeResolver(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("empty DNS resolver address")
	}
	if isDecimal(value) {
		return "", fmt.Errorf("invalid DNS resolver address %q", value)
	}
	if host, port, err := net.SplitHostPort(value); err == nil {
		if host == "" {
			return "", fmt.Errorf("invalid DNS resolver address %q", value)
		}
		if err := validatePort(port); err != nil {
			return "", err
		}
		return net.JoinHostPort(host, port), nil
	}
	if ip := net.ParseIP(value); ip != nil {
		return net.JoinHostPort(value, "53"), nil
	}
	if strings.Count(value, ":") == 0 {
		return net.JoinHostPort(value, "53"), nil
	}
	return "", fmt.Errorf("invalid DNS resolver address %q", value)
}

func normalizeDomain(value string) (string, error) {
	value = strings.ToLower(strings.Trim(strings.TrimSpace(value), "."))
	if value == "" {
		return "", errors.New("empty DNS tunnel domain")
	}
	if len(value) > 253 {
		return "", fmt.Errorf("DNS tunnel domain too long: %d", len(value))
	}
	labels := strings.Split(value, ".")
	for _, label := range labels {
		if label == "" {
			return "", fmt.Errorf("invalid DNS tunnel domain %q", value)
		}
		if len(label) > 63 {
			return "", fmt.Errorf("DNS label %q too long", label)
		}
		for i, r := range label {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
				continue
			}
			if r == '-' && i > 0 && i < len(label)-1 {
				continue
			}
			return "", fmt.Errorf("invalid DNS label %q", label)
		}
	}
	return value, nil
}

func validatePort(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < 0 || port > 65535 {
		return fmt.Errorf("invalid DNS port %q", value)
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
