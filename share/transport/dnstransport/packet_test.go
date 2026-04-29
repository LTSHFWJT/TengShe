package dnstransport

import (
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestFrameCodec(t *testing.T) {
	in := frame{
		Type:      frameDATA,
		SessionID: 10,
		SenderID:  20,
		StreamID:  2,
		PacketID:  30,
		Seq:       40,
		Ack:       39,
		FragID:    3,
		FragIndex: 1,
		FragTotal: 2,
		Payload:   []byte("payload"),
	}
	raw, err := encodeFrame(in)
	if err != nil {
		t.Fatalf("encodeFrame error: %v", err)
	}
	out, err := decodeFrame(raw)
	if err != nil {
		t.Fatalf("decodeFrame error: %v", err)
	}
	if out.Type != in.Type || out.SessionID != in.SessionID || out.SenderID != in.SenderID || out.StreamID != in.StreamID || out.PacketID != in.PacketID || out.Seq != in.Seq || out.Ack != in.Ack || out.FragID != in.FragID || out.FragIndex != in.FragIndex || out.FragTotal != in.FragTotal || string(out.Payload) != string(in.Payload) {
		t.Fatalf("decoded frame = %+v, want %+v", out, in)
	}
}

func TestDNSMessageRoundTrip(t *testing.T) {
	payload, err := encodeFrame(frame{Type: framePOLL, SessionID: 1})
	if err != nil {
		t.Fatalf("encodeFrame error: %v", err)
	}
	queryRaw, queryID, err := buildQuery("t.example", payload, DefaultConfig())
	if err != nil {
		t.Fatalf("buildQuery error: %v", err)
	}
	query, err := parseQuery(queryRaw, "t.example")
	if err != nil {
		t.Fatalf("parseQuery error: %v", err)
	}
	if query.ID != queryID {
		t.Fatalf("query id = %d, want %d", query.ID, queryID)
	}
	responseRaw, err := buildResponse(query, payload, DefaultConfig())
	if err != nil {
		t.Fatalf("buildResponse error: %v", err)
	}
	responsePayload, err := parseResponse(responseRaw, queryID)
	if err != nil {
		t.Fatalf("parseResponse error: %v", err)
	}
	if string(responsePayload) != string(payload) {
		t.Fatalf("response payload mismatch")
	}
}

func TestDNSQueryIncludesEDNS0(t *testing.T) {
	payload, err := encodeFrame(frame{Type: framePOLL, SessionID: 1})
	if err != nil {
		t.Fatalf("encodeFrame error: %v", err)
	}
	raw, _, err := buildQuery("t.example", payload, DefaultConfig())
	if err != nil {
		t.Fatalf("buildQuery error: %v", err)
	}
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil {
		t.Fatalf("Unpack query error: %v", err)
	}
	if len(msg.Additionals) != 1 || msg.Additionals[0].Header.Type != dnsmessage.TypeOPT {
		t.Fatalf("query additionals = %+v, want one OPT resource", msg.Additionals)
	}
}

func TestDNSNXDOMAIN(t *testing.T) {
	name := dnsmessage.MustNewName("www.example.")
	query := dnsQuery{
		ID: 7,
		Question: dnsmessage.Question{
			Name:  name,
			Type:  dnsmessage.TypeA,
			Class: dnsmessage.ClassINET,
		},
	}
	raw, err := buildNXDOMAIN(query)
	if err != nil {
		t.Fatalf("buildNXDOMAIN error: %v", err)
	}
	var msg dnsmessage.Message
	if err := msg.Unpack(raw); err != nil {
		t.Fatalf("Unpack NXDOMAIN error: %v", err)
	}
	if !msg.Response || msg.RCode != dnsmessage.RCodeNameError {
		t.Fatalf("NXDOMAIN header = %+v", msg.Header)
	}
}
