package websockettransport

import "testing"

func TestNormalizeListenAddress(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "port", in: "8080", want: "ws://0.0.0.0:8080/tengshe"},
		{name: "host port path", in: "127.0.0.1:8080/t", want: "ws://127.0.0.1:8080/t"},
		{name: "ws url", in: "ws://127.0.0.1:8080/t", want: "ws://127.0.0.1:8080/t"},
		{name: "wss url", in: "wss://127.0.0.1:8443/t", want: "wss://127.0.0.1:8443/t"},
	}

	for _, tc := range tests {
		got, err := NormalizeListenAddress(tc.in)
		if err != nil {
			t.Fatalf("%s NormalizeListenAddress error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s NormalizeListenAddress = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestNormalizeDialAddress(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "host port", in: "127.0.0.1:8080", want: "ws://127.0.0.1:8080/tengshe"},
		{name: "host port path", in: "127.0.0.1:8080/t", want: "ws://127.0.0.1:8080/t"},
		{name: "ws url", in: "ws://127.0.0.1:8080/t", want: "ws://127.0.0.1:8080/t"},
		{name: "wss url", in: "wss://example.com:8443/t", want: "wss://example.com:8443/t"},
	}

	for _, tc := range tests {
		got, err := NormalizeDialAddress(tc.in)
		if err != nil {
			t.Fatalf("%s NormalizeDialAddress error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s NormalizeDialAddress = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestNormalizeAddressRejectsInvalidInputs(t *testing.T) {
	tests := []string{
		"http://127.0.0.1:8080/t",
		"ws://127.0.0.1:0/t",
		"ws://127.0.0.1/t",
		"127.0.0.1",
	}

	for _, in := range tests {
		if _, err := NormalizeDialAddress(in); err == nil {
			t.Fatalf("NormalizeDialAddress(%q) unexpectedly succeeded", in)
		}
	}
}

func TestNormalizeListenAddressRejectsInvalidPort(t *testing.T) {
	if _, err := NormalizeListenAddress("70000"); err == nil {
		t.Fatal("NormalizeListenAddress with invalid port unexpectedly succeeded")
	}
}
