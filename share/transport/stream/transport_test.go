package stream

import "testing"

func TestNormalizeProtocolDefaultsToTCP(t *testing.T) {
	got, err := NormalizeProtocol("")
	if err != nil {
		t.Fatalf("NormalizeProtocol empty error: %v", err)
	}
	if got != ProtocolTCP {
		t.Fatalf("NormalizeProtocol empty = %q, want %q", got, ProtocolTCP)
	}
}

func TestRegistryRejectsUnsupportedProtocol(t *testing.T) {
	if _, err := Get("unknown"); err == nil {
		t.Fatal("Get unsupported protocol unexpectedly succeeded")
	}
}

func TestTCPNormalizeListenAddress(t *testing.T) {
	got, err := NormalizeListenAddress(ProtocolTCP, "127.0.0.1:1080")
	if err != nil {
		t.Fatalf("NormalizeListenAddress tcp error: %v", err)
	}
	if got != "127.0.0.1:1080" {
		t.Fatalf("NormalizeListenAddress tcp = %q, want 127.0.0.1:1080", got)
	}
}

func TestICMPNormalizeDialAddressIgnoresPort(t *testing.T) {
	got, err := NormalizeDialAddress(ProtocolICMP, "127.0.0.1:9999")
	if err != nil {
		t.Fatalf("NormalizeDialAddress icmp error: %v", err)
	}
	if got != "127.0.0.1" {
		t.Fatalf("NormalizeDialAddress icmp = %q, want 127.0.0.1", got)
	}
}

func TestDNSNormalizeAddresses(t *testing.T) {
	listen, err := NormalizeListenAddress(ProtocolDNS, "5353/t.example")
	if err != nil {
		t.Fatalf("NormalizeListenAddress dns error: %v", err)
	}
	if listen != "0.0.0.0:5353/t.example" {
		t.Fatalf("NormalizeListenAddress dns = %q, want 0.0.0.0:5353/t.example", listen)
	}

	dial, err := NormalizeDialAddress(ProtocolDNS, "t.example@127.0.0.1:5353")
	if err != nil {
		t.Fatalf("NormalizeDialAddress dns error: %v", err)
	}
	if dial != "t.example@127.0.0.1:5353" {
		t.Fatalf("NormalizeDialAddress dns = %q, want t.example@127.0.0.1:5353", dial)
	}
}
