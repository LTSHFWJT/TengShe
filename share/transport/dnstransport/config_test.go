package dnstransport

import "testing"

func TestConfigForcesHEXCodec(t *testing.T) {
	config := normalizeConfig(Config{Codec: "base32"})
	if config.Codec != "hex" {
		t.Fatalf("Codec = %q, want hex", config.Codec)
	}
}
