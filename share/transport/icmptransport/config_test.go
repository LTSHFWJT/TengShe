package icmptransport

import (
	"testing"
	"time"
)

func TestDefaultConfigFromEnv(t *testing.T) {
	t.Setenv("TENGSHE_ICMP_MTU", "512")
	t.Setenv("TENGSHE_ICMP_WINDOW", "16")
	t.Setenv("TENGSHE_ICMP_INITIAL_WINDOW", "8")
	t.Setenv("TENGSHE_ICMP_CLOSE_TIMEOUT", "150ms")
	t.Setenv("TENGSHE_ICMP_RETRANSMIT_MIN", "25")

	config := DefaultConfigFromEnv()
	if config.PayloadMTU != 512 {
		t.Fatalf("PayloadMTU = %d, want 512", config.PayloadMTU)
	}
	if config.SendWindow != 16 {
		t.Fatalf("SendWindow = %d, want 16", config.SendWindow)
	}
	if config.InitialWindow != 8 {
		t.Fatalf("InitialWindow = %d, want 8", config.InitialWindow)
	}
	if config.CloseTimeout != 150*time.Millisecond {
		t.Fatalf("CloseTimeout = %s, want 150ms", config.CloseTimeout)
	}
	if config.RetransmitMin != 25*time.Millisecond {
		t.Fatalf("RetransmitMin = %s, want 25ms", config.RetransmitMin)
	}
}
