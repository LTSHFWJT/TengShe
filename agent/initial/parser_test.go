package initial

import (
	"reflect"
	"testing"
)

func TestNormalizeProtocolFlagArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "short protocol unchanged",
			in:   []string{"-p", "icmp", "-c", "127.0.0.1"},
			want: []string{"-p", "icmp", "-c", "127.0.0.1"},
		},
		{
			name: "legacy separated transport",
			in:   []string{"--transport", "icmp", "-c", "127.0.0.1"},
			want: []string{"-p", "icmp", "-c", "127.0.0.1"},
		},
		{
			name: "legacy equals transport",
			in:   []string{"--transport=icmp", "-l", "0.0.0.0"},
			want: []string{"-p=icmp", "-l", "0.0.0.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeProtocolFlagArgs(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizeProtocolFlagArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
