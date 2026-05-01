package smbtransport

import (
	"testing"
)

func TestNormalizeListenAddress(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain name", in: "tengshe", want: `\\.\pipe\tengshe`},
		{name: "pipe prefix", in: "pipe:tengshe", want: `\\.\pipe\tengshe`},
		{name: "local url", in: "pipe://./tengshe", want: `\\.\pipe\tengshe`},
		{name: "native local", in: `\\.\pipe\tengshe`, want: `\\.\pipe\tengshe`},
		{name: "slash group", in: "pipe:tengshe/node2", want: `\\.\pipe\tengshe\node2`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeListenAddress(tt.in)
			if err != nil {
				t.Fatalf("NormalizeListenAddress error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeListenAddress(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeDialAddress(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain name", in: "tengshe", want: `\\.\pipe\tengshe`},
		{name: "local url", in: "pipe://./tengshe", want: `\\.\pipe\tengshe`},
		{name: "remote url", in: "pipe://host/tengshe", want: `\\host\pipe\tengshe`},
		{name: "remote native", in: `\\host\pipe\tengshe`, want: `\\host\pipe\tengshe`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeDialAddress(tt.in)
			if err != nil {
				t.Fatalf("NormalizeDialAddress error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeDialAddress(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeAddressRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		listen bool
	}{
		{name: "empty listen", value: "", listen: true},
		{name: "remote listen", value: `\\host\pipe\tengshe`, listen: true},
		{name: "unsupported scheme", value: "tcp://127.0.0.1/tengshe", listen: false},
		{name: "shared file", value: "file:/mnt/share/tengshe", listen: false},
		{name: "host with port", value: "pipe://host:1445/tengshe", listen: false},
		{name: "dual mode", value: "pipe://host/tengshe?mode=dual", listen: false},
		{name: "parent segment", value: "pipe:tengshe/../bad", listen: true},
		{name: "bad chars", value: `pipe:tengshe|bad`, listen: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.listen {
				_, err = NormalizeListenAddress(tt.value)
			} else {
				_, err = NormalizeDialAddress(tt.value)
			}
			if err == nil {
				t.Fatalf("expected error for %q", tt.value)
			}
		})
	}
}

func TestTransportName(t *testing.T) {
	if got := (Addr{}).Network(); got != "smb" {
		t.Fatalf("Addr.Network() = %q, want smb", got)
	}
}
