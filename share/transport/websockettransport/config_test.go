package websockettransport

import (
	"net/http"
	"testing"
)

func TestTransportNameIsCanonicalWS(t *testing.T) {
	if got := (Addr{}).Network(); got != "ws" {
		t.Fatalf("Addr.Network() = %q, want ws", got)
	}
}

func TestParseHeadersEnvFiltersReservedHeaders(t *testing.T) {
	headers := parseHeadersEnv("X-Trace=ok;Origin=http://bad;Sec-WebSocket-Protocol=bad;X-Unsafe=line\r\nbad")
	if got := headers.Get("X-Trace"); got != "ok" {
		t.Fatalf("X-Trace = %q, want ok", got)
	}
	if got := headers.Get("Origin"); got != "" {
		t.Fatalf("Origin was not filtered: %q", got)
	}
	if got := headers.Get("Sec-Websocket-Protocol"); got != "" {
		t.Fatalf("Sec-WebSocket-Protocol was not filtered: %q", got)
	}
	if got := headers.Get("X-Unsafe"); got != "" {
		t.Fatalf("unsafe header value was not filtered: %q", got)
	}
}

func TestCloneHeaderFiltersReservedHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Trace", "ok")
	headers.Set("Connection", "close")
	headers.Set("Origin", "http://bad")

	cloned := cloneHeader(headers)
	if got := cloned.Get("X-Trace"); got != "ok" {
		t.Fatalf("X-Trace = %q, want ok", got)
	}
	if got := cloned.Get("Connection"); got != "" {
		t.Fatalf("Connection was not filtered: %q", got)
	}
	if got := cloned.Get("Origin"); got != "" {
		t.Fatalf("Origin was not filtered: %q", got)
	}
}
