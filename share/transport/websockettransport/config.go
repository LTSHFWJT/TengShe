package websockettransport

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	TransportName = "ws"
	Subprotocol   = "tengshe-v1"

	defaultPath             = "/tengshe"
	defaultHandshakeTimeout = 10 * time.Second
	defaultMaxFramePayload  = 64 * 1024
	defaultAcceptBacklog    = 128
)

var reservedWebSocketHeaders = map[string]struct{}{
	"Connection":               {},
	"Host":                     {},
	"Origin":                   {},
	"Sec-Websocket-Accept":     {},
	"Sec-Websocket-Extensions": {},
	"Sec-Websocket-Key":        {},
	"Sec-Websocket-Origin":     {},
	"Sec-Websocket-Protocol":   {},
	"Sec-Websocket-Version":    {},
	"Upgrade":                  {},
}

type Config struct {
	Path             string
	HandshakeTimeout time.Duration
	MaxFramePayload  int
	AcceptBacklog    int
	Origin           string
	Host             string
	Headers          http.Header
}

func DefaultConfig() Config {
	return Config{
		Path:             defaultPath,
		HandshakeTimeout: defaultHandshakeTimeout,
		MaxFramePayload:  defaultMaxFramePayload,
		AcceptBacklog:    defaultAcceptBacklog,
		Origin:           "http://127.0.0.1/",
		Headers:          make(http.Header),
	}
}

func DefaultConfigFromEnv() Config {
	config := DefaultConfig()
	applyStringEnv("TENGSHE_WS_PATH", &config.Path)
	applyDurationEnv("TENGSHE_WS_HANDSHAKE_TIMEOUT", &config.HandshakeTimeout)
	applyIntEnv("TENGSHE_WS_MAX_FRAME", &config.MaxFramePayload)
	applyIntEnv("TENGSHE_WS_ACCEPT_BACKLOG", &config.AcceptBacklog)
	applyStringEnv("TENGSHE_WS_ORIGIN", &config.Origin)
	applyStringEnv("TENGSHE_WS_HOST", &config.Host)
	config.Headers = parseHeadersEnv(os.Getenv("TENGSHE_WS_HEADERS"))
	return normalizeConfig(config)
}

func normalizeConfig(config Config) Config {
	config.Path = normalizePath(config.Path)
	if config.HandshakeTimeout <= 0 {
		config.HandshakeTimeout = defaultHandshakeTimeout
	}
	if config.MaxFramePayload <= 0 {
		config.MaxFramePayload = defaultMaxFramePayload
	}
	if config.AcceptBacklog <= 0 {
		config.AcceptBacklog = defaultAcceptBacklog
	}
	config.Origin = strings.TrimSpace(config.Origin)
	if config.Origin == "" {
		config.Origin = "http://127.0.0.1/"
	}
	config.Host = strings.TrimSpace(config.Host)
	if config.Headers == nil {
		config.Headers = make(http.Header)
	}
	return config
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return defaultPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func parseHeadersEnv(value string) http.Header {
	headers := make(http.Header)
	for _, item := range strings.Split(value, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key, val, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		key = http.CanonicalHeaderKey(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		if key == "" || val == "" || isReservedWebSocketHeader(key) || !isSafeHeaderValue(val) {
			continue
		}
		headers.Add(key, val)
	}
	return headers
}

func isReservedWebSocketHeader(key string) bool {
	key = http.CanonicalHeaderKey(strings.TrimSpace(key))
	_, ok := reservedWebSocketHeaders[key]
	return ok
}

func isSafeHeaderValue(value string) bool {
	return !strings.ContainsAny(value, "\r\n")
}

func applyIntEnv(name string, dst *int) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		*dst = parsed
	}
}

func applyStringEnv(name string, dst *string) {
	value := strings.TrimSpace(os.Getenv(name))
	if value != "" {
		*dst = value
	}
}

func applyDurationEnv(name string, dst *time.Duration) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		*dst = parsed
		return
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		*dst = time.Duration(parsed) * time.Millisecond
	}
}
