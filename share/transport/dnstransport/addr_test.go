package dnstransport

import "testing"

func TestNormalizeListenAddress(t *testing.T) {
	got, err := NormalizeListenAddress("5353/t.example")
	if err != nil {
		t.Fatalf("NormalizeListenAddress error: %v", err)
	}
	if got != "0.0.0.0:5353/t.example" {
		t.Fatalf("NormalizeListenAddress = %q, want 0.0.0.0:5353/t.example", got)
	}
}

func TestNormalizeDialAddress(t *testing.T) {
	got, err := NormalizeDialAddress("t.example@127.0.0.1:5353")
	if err != nil {
		t.Fatalf("NormalizeDialAddress error: %v", err)
	}
	if got != "t.example@127.0.0.1:5353" {
		t.Fatalf("NormalizeDialAddress = %q, want t.example@127.0.0.1:5353", got)
	}
}

func TestNormalizeDialAddressDefaultsResolver(t *testing.T) {
	t.Setenv("TENGSHE_DNS_RESOLVER", "203.0.113.10:5353")
	got, err := NormalizeDialAddress("t.example")
	if err != nil {
		t.Fatalf("NormalizeDialAddress error: %v", err)
	}
	if got != "t.example@203.0.113.10:5353" {
		t.Fatalf("NormalizeDialAddress = %q, want t.example@203.0.113.10:5353", got)
	}
}

func TestNormalizeResolverAddsPortToIPv6(t *testing.T) {
	got, err := normalizeResolver("2001:4860:4860::8888")
	if err != nil {
		t.Fatalf("normalizeResolver IPv6 error: %v", err)
	}
	if got != "[2001:4860:4860::8888]:53" {
		t.Fatalf("normalizeResolver IPv6 = %q, want [2001:4860:4860::8888]:53", got)
	}
}
