package smbtransport

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

const (
	localPipePrefix = `\\.\pipe\`
	pipePathPart    = `\pipe\`
	maxPipePathLen  = 240
)

type Addr struct {
	Path string
}

func (addr Addr) Network() string {
	return TransportName
}

func (addr Addr) String() string {
	if addr.Path == "" {
		return localPipePrefix
	}
	return addr.Path
}

func NormalizeListenAddress(value string) (string, error) {
	return normalizePipeAddress(value, true)
}

func NormalizeDialAddress(value string) (string, error) {
	return normalizePipeAddress(value, false)
}

func normalizePipeAddress(value string, listen bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if listen {
			return "", errors.New("empty SMB listen address")
		}
		return "", errors.New("empty SMB dial address")
	}
	if strings.HasPrefix(strings.ToLower(value), "pipe://") {
		return normalizePipeURL(value, listen)
	}
	if strings.HasPrefix(strings.ToLower(value), "file:") {
		return "", errors.New("SMB shared-file transport is not supported")
	}
	if strings.Contains(value, "://") {
		return "", fmt.Errorf("unsupported SMB address scheme in %q", value)
	}
	if strings.HasPrefix(value, `\\`) {
		return normalizeNativePipePath(value, listen)
	}
	if strings.HasPrefix(strings.ToLower(value), "pipe:") {
		value = strings.TrimSpace(value[len("pipe:"):])
	}
	return buildNativePipePath(".", value, listen)
}

func normalizePipeURL(value string, listen bool) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "pipe" {
		return "", fmt.Errorf("unsupported SMB pipe scheme %q", parsed.Scheme)
	}
	if parsed.User != nil {
		return "", errors.New("SMB pipe address must not include user info")
	}
	mode := strings.ToLower(strings.TrimSpace(parsed.Query().Get("mode")))
	if mode != "" && mode != "duplex" {
		return "", fmt.Errorf("SMB pipe mode %q is reserved but not implemented", mode)
	}
	host := strings.TrimSpace(parsed.Host)
	name, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil {
		return "", err
	}
	if listen && host != "" && host != "." {
		return "", errors.New("SMB listen address must use local pipe host")
	}
	if !listen && host == "" {
		return "", fmt.Errorf("invalid SMB dial address %q: empty host", value)
	}
	return buildNativePipePath(host, name, listen)
}

func normalizeNativePipePath(value string, listen bool) (string, error) {
	value = strings.TrimSpace(value)
	host, name, err := splitNativePipePath(value)
	if err != nil {
		return "", err
	}
	if listen && host != "." {
		return "", errors.New("SMB listen address must use local pipe path \\\\.\\pipe\\name")
	}
	return buildNativePipePath(host, name, listen)
}

func splitNativePipePath(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(value), `\\`) {
		return "", "", fmt.Errorf("invalid SMB pipe path %q", value)
	}
	rest := value[2:]
	idx := strings.Index(strings.ToLower(rest), pipePathPart)
	if idx <= 0 {
		return "", "", fmt.Errorf("invalid SMB pipe path %q: expected \\\\host\\pipe\\name", value)
	}
	return rest[:idx], rest[idx+len(pipePathPart):], nil
}

func isLocalPipeHost(host string) bool {
	return host == "."
}

func buildNativePipePath(host, name string, listen bool) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		if listen {
			host = "."
		} else {
			return "", errors.New("empty SMB pipe host")
		}
	}
	if listen {
		host = "."
	}
	if host == "." || strings.EqualFold(host, "localhost") {
		host = "."
	}
	if err := validatePipeHost(host); err != nil {
		return "", err
	}
	name = normalizePipeName(name)
	if err := validatePipeName(name); err != nil {
		return "", err
	}
	path := `\\` + host + pipePathPart + name
	if len(path) > maxPipePathLen {
		return "", fmt.Errorf("SMB pipe path is too long: %d > %d", len(path), maxPipePathLen)
	}
	return path, nil
}

func normalizePipeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, `/\`)
	name = strings.ReplaceAll(name, "/", `\`)
	for strings.Contains(name, `\\`) {
		name = strings.ReplaceAll(name, `\\`, `\`)
	}
	return name
}

func validatePipeHost(host string) error {
	if host == "" {
		return errors.New("empty SMB pipe host")
	}
	if strings.ContainsAny(host, `/\`) || strings.Contains(host, "..") || hasControl(host) || strings.ContainsAny(host, " \t\r\n") {
		return fmt.Errorf("invalid SMB pipe host %q", host)
	}
	if strings.Contains(host, ":") {
		return fmt.Errorf("invalid SMB pipe host %q: use TENGSHE_SMB_PORT for non-default ports", host)
	}
	return nil
}

func validatePipeName(name string) error {
	if name == "" {
		return errors.New("empty SMB pipe name")
	}
	if hasControl(name) {
		return fmt.Errorf("invalid SMB pipe name %q: control characters are not allowed", name)
	}
	for _, segment := range strings.Split(name, `\`) {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("invalid SMB pipe name segment %q", segment)
		}
		if strings.ContainsAny(segment, `:*?"<>|`) {
			return fmt.Errorf("invalid SMB pipe name segment %q", segment)
		}
	}
	return nil
}

func hasControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}
