//go:build !windows

package dnstransport

import "testing"

func TestResolvConfNameservers(t *testing.T) {
	got := resolvConfNameservers(`
# comment
nameserver 192.0.2.53
nameserver 2001:4860:4860::8888
search example.test
`)
	want := []string{"192.0.2.53", "2001:4860:4860::8888"}
	if len(got) != len(want) {
		t.Fatalf("resolvConfNameservers length = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resolvConfNameservers[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
