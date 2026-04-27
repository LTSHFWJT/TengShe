package process

import (
	"testing"

	"TengShe/protocol"
)

func TestChangeRouteSingleHop(t *testing.T) {
	header := &protocol.Header{
		Route:    "child",
		RouteLen: uint32(len("child")),
	}

	next := changeRoute(header)

	if next != "child" {
		t.Fatalf("next hop = %q, want child", next)
	}
	if header.Route != "" {
		t.Fatalf("remaining route = %q, want empty", header.Route)
	}
	if header.RouteLen != 0 {
		t.Fatalf("remaining route length = %d, want 0", header.RouteLen)
	}
}

func TestChangeRouteMultiHop(t *testing.T) {
	header := &protocol.Header{
		Route:    "child:grand",
		RouteLen: uint32(len("child:grand")),
	}

	next := changeRoute(header)

	if next != "child" {
		t.Fatalf("next hop = %q, want child", next)
	}
	if header.Route != "grand" {
		t.Fatalf("remaining route = %q, want grand", header.Route)
	}
	if header.RouteLen != uint32(len("grand")) {
		t.Fatalf("remaining route length = %d, want %d", header.RouteLen, len("grand"))
	}
}

func TestParseDownstreamRoute(t *testing.T) {
	tests := []struct {
		name          string
		route         string
		wantNext      string
		wantRemaining string
	}{
		{name: "empty", route: "", wantNext: "", wantRemaining: ""},
		{name: "single", route: "child", wantNext: "child", wantRemaining: ""},
		{name: "multi", route: "child:grand:leaf", wantNext: "child", wantRemaining: "grand:leaf"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseDownstreamRoute(tc.route)
			if got.NextHop != tc.wantNext || got.Remaining != tc.wantRemaining {
				t.Fatalf("parse route = %+v, want next=%q remaining=%q", got, tc.wantNext, tc.wantRemaining)
			}
		})
	}
}
