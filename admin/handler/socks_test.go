package handler

import "testing"

func TestNewSocksDefaultPortUsesIPv4Wildcard(t *testing.T) {
	socks := NewSocks("8889")

	if socks.Addr != "0.0.0.0" || socks.Port != "8889" {
		t.Fatalf("socks bind = %s:%s, want 0.0.0.0:8889", socks.Addr, socks.Port)
	}
	if got := socks.listenNetwork(); got != "tcp4" {
		t.Fatalf("listen network = %s, want tcp4", got)
	}
	if got := socks.listenAddr(); got != "0.0.0.0:8889" {
		t.Fatalf("listen addr = %s, want 0.0.0.0:8889", got)
	}
}

func TestNewSocksIPv4BindUsesTCP4(t *testing.T) {
	socks := NewSocks("127.0.0.1:8889")

	if socks.Addr != "127.0.0.1" || socks.Port != "8889" {
		t.Fatalf("socks bind = %s:%s, want 127.0.0.1:8889", socks.Addr, socks.Port)
	}
	if got := socks.listenNetwork(); got != "tcp4" {
		t.Fatalf("listen network = %s, want tcp4", got)
	}
}

func TestNewSocksIPv6BindUsesTCP6(t *testing.T) {
	socks := NewSocks("[::1]:8889")

	if socks.Addr != "::1" || socks.Port != "8889" {
		t.Fatalf("socks bind = %s:%s, want ::1:8889", socks.Addr, socks.Port)
	}
	if got := socks.listenNetwork(); got != "tcp6" {
		t.Fatalf("listen network = %s, want tcp6", got)
	}
	if got := socks.listenAddr(); got != "[::1]:8889" {
		t.Fatalf("listen addr = %s, want [::1]:8889", got)
	}
}
