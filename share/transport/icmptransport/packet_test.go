package icmptransport

import (
	"bytes"
	"testing"
)

func TestFrameEncodeDecode(t *testing.T) {
	payload := []byte("hello tengshe")
	in := frame{
		Type:       frameDATA,
		Flags:      7,
		SessionID:  42,
		SenderID:   99,
		StreamID:   2,
		Seq:        100,
		Ack:        90,
		FragID:     3,
		FragIndex:  1,
		FragTotal:  2,
		PayloadLen: uint16(len(payload)),
		Payload:    payload,
	}

	raw, err := encodeFrame(in)
	if err != nil {
		t.Fatalf("encodeFrame error: %v", err)
	}
	got, err := decodeFrame(raw)
	if err != nil {
		t.Fatalf("decodeFrame error: %v", err)
	}

	if got.Type != in.Type ||
		got.Flags != in.Flags ||
		got.SessionID != in.SessionID ||
		got.SenderID != in.SenderID ||
		got.StreamID != in.StreamID ||
		got.Seq != in.Seq ||
		got.Ack != in.Ack ||
		got.FragID != in.FragID ||
		got.FragIndex != in.FragIndex ||
		got.FragTotal != in.FragTotal {
		t.Fatalf("decoded frame mismatch: got %+v want %+v", got, in)
	}
	if !bytes.Equal(got.Payload, payload) {
		t.Fatalf("decoded payload = %q, want %q", got.Payload, payload)
	}
}

func TestDecodeFrameRejectsInvalidInput(t *testing.T) {
	if _, err := decodeFrame([]byte("short")); err == nil {
		t.Fatal("decodeFrame accepted short frame")
	}

	raw, err := encodeFrame(frame{Type: frameDATA, SessionID: 1})
	if err != nil {
		t.Fatalf("encodeFrame error: %v", err)
	}
	raw[0] = 0
	if _, err := decodeFrame(raw); err == nil {
		t.Fatal("decodeFrame accepted bad magic")
	}

	raw, err = encodeFrame(frame{Type: frameType(255), SessionID: 1})
	if err != nil {
		t.Fatalf("encodeFrame invalid type setup error: %v", err)
	}
	if _, err := decodeFrame(raw); err == nil {
		t.Fatal("decodeFrame accepted invalid frame type")
	}
}

func TestNormalizeICMPAddresses(t *testing.T) {
	listen, err := NormalizeListenAddress("1111")
	if err != nil {
		t.Fatalf("NormalizeListenAddress numeric error: %v", err)
	}
	if listen != "0.0.0.0" {
		t.Fatalf("numeric listen address = %q, want 0.0.0.0", listen)
	}

	peer, err := NormalizePeerAddress("127.0.0.1:9000")
	if err != nil {
		t.Fatalf("NormalizePeerAddress hostport error: %v", err)
	}
	if peer != "127.0.0.1" {
		t.Fatalf("peer = %q, want 127.0.0.1", peer)
	}
}

func TestSplitPayload(t *testing.T) {
	chunks := splitPayload([]byte("abcdef"), 2)
	if len(chunks) != 3 {
		t.Fatalf("chunks len = %d, want 3", len(chunks))
	}
	if string(chunks[0]) != "ab" || string(chunks[1]) != "cd" || string(chunks[2]) != "ef" {
		t.Fatalf("unexpected chunks: %q", chunks)
	}
}
