package smbtransport

import (
	"testing"
	"time"
)

func TestNormalizeConfig(t *testing.T) {
	config := normalizeConfig(Config{
		DialTimeout:   -time.Second,
		IOTimeout:     -time.Second,
		BufferSize:    -1,
		MaxChunk:      -1,
		AcceptBacklog: 999,
		RetryInterval: -time.Second,
	})
	if config.SMBPort != defaultSMBPort {
		t.Fatalf("SMBPort = %d, want %d", config.SMBPort, defaultSMBPort)
	}
	if !config.SMBNullSession {
		t.Fatal("SMBNullSession = false, want true when user and password are empty")
	}
	if config.DialTimeout != defaultDialTimeout {
		t.Fatalf("DialTimeout = %s, want %s", config.DialTimeout, defaultDialTimeout)
	}
	if config.IOTimeout != 0 {
		t.Fatalf("IOTimeout = %s, want 0", config.IOTimeout)
	}
	if config.BufferSize != defaultBufferSize {
		t.Fatalf("BufferSize = %d, want %d", config.BufferSize, defaultBufferSize)
	}
	if config.MaxChunk != defaultMaxChunk {
		t.Fatalf("MaxChunk = %d, want %d", config.MaxChunk, defaultMaxChunk)
	}
	if config.AcceptBacklog != 255 {
		t.Fatalf("AcceptBacklog = %d, want 255", config.AcceptBacklog)
	}
	if config.RetryInterval != defaultRetryInterval {
		t.Fatalf("RetryInterval = %s, want %s", config.RetryInterval, defaultRetryInterval)
	}
}
