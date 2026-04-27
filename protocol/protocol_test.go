package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	tengcrypto "TengShe/crypto"
)

func TestSetUpDownStream(t *testing.T) {
	oldUpstream, oldDownstream := Upstream, Downstream
	defer func() {
		Upstream, Downstream = oldUpstream, oldDownstream
	}()

	tests := []struct {
		name           string
		upstream       string
		downstream     string
		wantUpstream   string
		wantDownstream string
	}{
		{name: "raw", upstream: "raw", downstream: "raw", wantUpstream: "raw", wantDownstream: "raw"},
		{name: "http", upstream: "http", downstream: "http", wantUpstream: "http", wantDownstream: "http"},
		{name: "ws", upstream: "ws", downstream: "ws", wantUpstream: "ws", wantDownstream: "ws"},
		{name: "fallback", upstream: "tcp", downstream: "tls", wantUpstream: "raw", wantDownstream: "raw"},
	}

	for _, tc := range tests {
		SetUpDownStream(tc.upstream, tc.downstream)

		if Upstream != tc.wantUpstream {
			t.Fatalf("%s upstream = %q, want %q", tc.name, Upstream, tc.wantUpstream)
		}
		if Downstream != tc.wantDownstream {
			t.Fatalf("%s downstream = %q, want %q", tc.name, Downstream, tc.wantDownstream)
		}
	}
}

func TestNewProtoFactories(t *testing.T) {
	oldUpstream, oldDownstream := Upstream, Downstream
	defer func() {
		Upstream, Downstream = oldUpstream, oldDownstream
	}()

	for _, stream := range []string{"raw", "http", "ws"} {
		client, server := net.Pipe()
		param := &NegParam{Domain: "example.test", Conn: client}

		SetUpDownStream(stream, "raw")
		assertProtoType(t, "upstream "+stream, NewUpProto(param), stream)

		SetUpDownStream("raw", stream)
		assertProtoType(t, "downstream "+stream, NewDownProto(param), stream)

		client.Close()
		server.Close()
	}
}

func assertProtoType(t *testing.T, name string, got Proto, stream string) {
	t.Helper()

	switch stream {
	case "raw":
		if _, ok := got.(*RawProto); !ok {
			t.Fatalf("%s proto type = %T, want *RawProto", name, got)
		}
	case "http":
		if _, ok := got.(*HTTPProto); !ok {
			t.Fatalf("%s proto type = %T, want *HTTPProto", name, got)
		}
	case "ws":
		wsProto, ok := got.(*WSProto)
		if !ok {
			t.Fatalf("%s proto type = %T, want *WSProto", name, got)
		}
		if wsProto.domain != "example.test" || wsProto.conn == nil {
			t.Fatalf("%s ws proto was not initialized with negotiation parameters", name)
		}
	default:
		t.Fatalf("unsupported stream %q", stream)
	}
}

func TestNewMessageFactories(t *testing.T) {
	oldUpstream, oldDownstream := Upstream, Downstream
	defer func() {
		Upstream, Downstream = oldUpstream, oldDownstream
	}()

	for _, stream := range []string{"raw", "http", "ws"} {
		client, server := net.Pipe()

		SetUpDownStream(stream, "raw")
		assertMessageType(t, "upstream "+stream, NewUpMsg(client, "secret", "NODE000001"), stream, client)

		SetUpDownStream("raw", stream)
		assertMessageType(t, "downstream "+stream, NewDownMsg(client, "secret", "NODE000001"), stream, client)

		client.Close()
		server.Close()
	}
}

func assertMessageType(t *testing.T, name string, got Message, stream string, conn net.Conn) {
	t.Helper()

	var raw *RawMessage
	switch stream {
	case "raw":
		msg, ok := got.(*RawMessage)
		if !ok {
			t.Fatalf("%s message type = %T, want *RawMessage", name, got)
		}
		raw = msg
	case "http":
		msg, ok := got.(*HTTPMessage)
		if !ok {
			t.Fatalf("%s message type = %T, want *HTTPMessage", name, got)
		}
		raw = msg.RawMessage
	case "ws":
		msg, ok := got.(*WSMessage)
		if !ok {
			t.Fatalf("%s message type = %T, want *WSMessage", name, got)
		}
		raw = msg.RawMessage
	default:
		t.Fatalf("unsupported stream %q", stream)
	}

	if raw == nil {
		t.Fatalf("%s raw message component is nil", name)
	}
	if raw.Conn != conn {
		t.Fatalf("%s conn was not assigned", name)
	}
	if raw.UUID != "NODE000001" {
		t.Fatalf("%s uuid = %q, want NODE000001", name, raw.UUID)
	}
	if !bytes.Equal(raw.CryptoSecret, tengcrypto.KeyPadding([]byte("secret"))) {
		t.Fatalf("%s crypto secret was not padded as expected", name)
	}
}

func TestRawMessageRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	secret := tengcrypto.KeyPadding([]byte("roundtrip-secret"))
	sender := "NODE000001"
	route := "NODE000002:NODE000003"
	body := &MyMemo{MemoLen: uint64(len("round trip memo")), Memo: "round trip memo"}
	header := &Header{
		Sender:      sender,
		Accepter:    ADMIN_UUID,
		MessageType: MYMEMO,
		RouteLen:    uint32(len(route)),
		Route:       route,
	}

	out := &RawMessage{UUID: sender, Conn: client, CryptoSecret: secret}
	ConstructMessage(out, header, body, false)

	done := make(chan struct{})
	go func() {
		out.SendMessage()
		close(done)
	}()

	in := &RawMessage{UUID: ADMIN_UUID, Conn: server, CryptoSecret: secret}
	gotHeader, gotBody, err := DestructMessage(in)
	if err != nil {
		t.Fatalf("destruct raw message: %v", err)
	}
	waitForSend(t, done)

	if gotHeader.Sender != sender || gotHeader.Accepter != ADMIN_UUID ||
		gotHeader.MessageType != MYMEMO || gotHeader.Route != route ||
		gotHeader.RouteLen != uint32(len(route)) {
		t.Fatalf("decoded header = %+v", gotHeader)
	}

	gotMemo, ok := gotBody.(*MyMemo)
	if !ok {
		t.Fatalf("decoded body type = %T, want *MyMemo", gotBody)
	}
	if gotMemo.Memo != body.Memo || gotMemo.MemoLen != body.MemoLen {
		t.Fatalf("decoded memo = %+v, want %+v", gotMemo, body)
	}
}

func TestRawMessagePassThrough(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	payload := []byte("already encoded payload")
	header := &Header{
		Sender:      "NODE000001",
		Accepter:    "TARGET0001",
		MessageType: SOCKSTCPDATA,
		RouteLen:    uint32(len("TARGET0001")),
		Route:       "TARGET0001",
	}

	out := &RawMessage{UUID: "NODE000001", Conn: client, CryptoSecret: tengcrypto.KeyPadding([]byte("secret"))}
	ConstructMessage(out, header, payload, true)

	done := make(chan struct{})
	go func() {
		out.SendMessage()
		close(done)
	}()

	in := &RawMessage{UUID: "OTHER00001", Conn: server, CryptoSecret: tengcrypto.KeyPadding([]byte("secret"))}
	gotHeader, gotPayload, err := DestructMessage(in)
	if err != nil {
		t.Fatalf("destruct pass-through message: %v", err)
	}
	waitForSend(t, done)

	if gotHeader.Accepter != header.Accepter || gotHeader.Route != header.Route {
		t.Fatalf("decoded pass-through header = %+v, want accepter %q route %q", gotHeader, header.Accepter, header.Route)
	}

	gotBytes, ok := gotPayload.([]byte)
	if !ok {
		t.Fatalf("pass-through payload type = %T, want []byte", gotPayload)
	}
	if !bytes.Equal(gotBytes, payload) {
		t.Fatalf("pass-through payload = %q, want %q", gotBytes, payload)
	}
}

func TestSendMessageHelperRoundTrip(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	secret := tengcrypto.KeyPadding([]byte("send-helper-secret"))
	header := &Header{
		Sender:      "NODE000001",
		Accepter:    ADMIN_UUID,
		MessageType: MYMEMO,
		RouteLen:    uint32(len(TEMP_ROUTE)),
		Route:       TEMP_ROUTE,
	}
	body := &MyMemo{MemoLen: uint64(len("helper memo")), Memo: "helper memo"}

	done := make(chan struct{})
	go func() {
		SendMessage(&RawMessage{UUID: "NODE000001", Conn: client, CryptoSecret: secret}, header, body, false)
		close(done)
	}()

	gotHeader, gotBody, err := DestructMessage(&RawMessage{UUID: ADMIN_UUID, Conn: server, CryptoSecret: secret})
	if err != nil {
		t.Fatalf("destruct helper message: %v", err)
	}
	waitForSend(t, done)

	if gotHeader.MessageType != MYMEMO || gotHeader.Route != TEMP_ROUTE {
		t.Fatalf("decoded header = %+v", gotHeader)
	}

	gotMemo, ok := gotBody.(*MyMemo)
	if !ok {
		t.Fatalf("decoded body type = %T, want *MyMemo", gotBody)
	}
	if gotMemo.Memo != body.Memo {
		t.Fatalf("memo = %q, want %q", gotMemo.Memo, body.Memo)
	}
}

func TestRawStreamDataRoundTripUsesExactPayloadLength(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	secret := tengcrypto.KeyPadding([]byte("stream-data-secret"))
	payload := []byte("stream payload")
	header := &Header{
		Sender:      "NODE000001",
		Accepter:    ADMIN_UUID,
		MessageType: FORWARDDATA,
		RouteLen:    uint32(len(TEMP_ROUTE)),
		Route:       TEMP_ROUTE,
	}
	body := &ForwardData{
		Seq:     42,
		DataLen: 999,
		Data:    payload,
	}

	done := make(chan struct{})
	go func() {
		SendMessage(&RawMessage{UUID: "NODE000001", Conn: client, CryptoSecret: secret}, header, body, false)
		close(done)
	}()

	gotHeader, gotBody, err := DestructMessage(&RawMessage{UUID: ADMIN_UUID, Conn: server, CryptoSecret: secret})
	if err != nil {
		t.Fatalf("destruct stream data: %v", err)
	}
	waitForSend(t, done)

	if gotHeader.MessageType != FORWARDDATA {
		t.Fatalf("decoded header type = %d, want FORWARDDATA", gotHeader.MessageType)
	}
	gotData, ok := gotBody.(*ForwardData)
	if !ok {
		t.Fatalf("decoded body type = %T, want *ForwardData", gotBody)
	}
	if gotData.Seq != body.Seq || gotData.DataLen != uint64(len(payload)) || !bytes.Equal(gotData.Data, payload) {
		t.Fatalf("decoded stream data = %+v, want seq=%d len=%d data=%q", gotData, body.Seq, len(payload), payload)
	}
}

func TestRawStreamDataRejectsMalformedPayload(t *testing.T) {
	dataBuf := make([]byte, 16+3)
	binary.BigEndian.PutUint64(dataBuf[:8], 1)
	binary.BigEndian.PutUint64(dataBuf[8:16], 4)
	copy(dataBuf[16:], []byte("abc"))

	_, err := decodeRawPayload(FORWARDDATA, dataBuf)
	if !errors.Is(err, errRawPayloadMalformed) {
		t.Fatalf("decode malformed stream data error = %v, want %v", err, errRawPayloadMalformed)
	}
}

func TestRawMessageRejectsOversizedRoute(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := writeRawFramePrefix(t, client, maxRawRouteLen+1, 0, false)
	_, _, err := DestructMessage(&RawMessage{Conn: server})
	waitForSend(t, done)

	if !errors.Is(err, errRawMessageTooLarge) {
		t.Fatalf("oversized route error = %v, want %v", err, errRawMessageTooLarge)
	}
}

func TestRawMessageRejectsOversizedData(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	done := writeRawFramePrefix(t, client, 0, maxRawDataLen+1, true)
	_, _, err := DestructMessage(&RawMessage{Conn: server})
	waitForSend(t, done)

	if !errors.Is(err, errRawMessageTooLarge) {
		t.Fatalf("oversized data error = %v, want %v", err, errRawMessageTooLarge)
	}
}

func writeRawFramePrefix(t *testing.T, conn net.Conn, routeLen uint32, dataLen uint64, includeDataLen bool) <-chan struct{} {
	t.Helper()

	var frame bytes.Buffer
	messageTypeBuf := make([]byte, 2)
	routeLenBuf := make([]byte, 4)

	binary.BigEndian.PutUint16(messageTypeBuf, MYMEMO)
	binary.BigEndian.PutUint32(routeLenBuf, routeLen)

	frame.Write([]byte("NODE000001"))
	frame.Write([]byte(ADMIN_UUID))
	frame.Write(messageTypeBuf)
	frame.Write(routeLenBuf)

	if includeDataLen {
		dataLenBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(dataLenBuf, dataLen)
		frame.Write(dataLenBuf)
	}

	done := make(chan struct{})
	go func() {
		conn.Write(frame.Bytes())
		close(done)
	}()

	return done
}

func waitForSend(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message send")
	}
}
