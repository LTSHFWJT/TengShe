package dnstransport

import "testing"

func TestQueryNameCodec(t *testing.T) {
	payload := []byte("hello dns")
	name, err := encodeQueryName(payload, "t.example", Config{LabelMaxLen: 8, QueryMaxLen: 220})
	if err != nil {
		t.Fatalf("encodeQueryName error: %v", err)
	}
	if name != "ts.68656c6c.6f20646e.73.t.example." {
		t.Fatalf("encoded name = %q, want hex labels", name)
	}
	decoded, err := decodeQueryName(name, "t.example")
	if err != nil {
		t.Fatalf("decodeQueryName error: %v", err)
	}
	if string(decoded) != string(payload) {
		t.Fatalf("decoded = %q, want %q", decoded, payload)
	}
}

func TestMaxPayloadMTUForDomainCapsHEXQuery(t *testing.T) {
	config := DefaultConfig()
	config.PayloadMTU = 180
	maxPayload, err := maxPayloadMTUForDomain("t.example", config)
	if err != nil {
		t.Fatalf("maxPayloadMTUForDomain error: %v", err)
	}
	if maxPayload <= 0 || maxPayload >= config.PayloadMTU {
		t.Fatalf("maxPayloadMTUForDomain = %d, want a positive cap below %d", maxPayload, config.PayloadMTU)
	}
	if _, err := encodeQueryName(make([]byte, frameHeaderSize+maxPayload), "t.example", config); err != nil {
		t.Fatalf("capped payload should fit: %v", err)
	}
}

func TestTextPayloadCodec(t *testing.T) {
	payload := []byte("hello txt")
	chunks := encodeTextPayload(payload)
	decoded, err := decodeTextPayload(chunks)
	if err != nil {
		t.Fatalf("decodeTextPayload error: %v", err)
	}
	if string(decoded) != string(payload) {
		t.Fatalf("decoded = %q, want %q", decoded, payload)
	}
}
