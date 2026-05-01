//go:build !windows

package smbtransport

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedOnNonWindows(t *testing.T) {
	_, err := ListenConfig(context.Background(), "tengshe", DefaultConfig())
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ListenConfig error = %v, want ErrUnsupported", err)
	}

	_, err = DialContext(context.Background(), "pipe://./tengshe", "", DefaultConfig())
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("DialContext error = %v, want ErrUnsupported", err)
	}
}
