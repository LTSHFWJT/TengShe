package protocol

import (
	"bytes"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

type recordingConn struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (conn *recordingConn) Read(_ []byte) (int, error)         { return 0, nil }
func (conn *recordingConn) Close() error                       { return nil }
func (conn *recordingConn) LocalAddr() net.Addr                { return dummyAddr("local") }
func (conn *recordingConn) RemoteAddr() net.Addr               { return dummyAddr("remote") }
func (conn *recordingConn) SetDeadline(_ time.Time) error      { return nil }
func (conn *recordingConn) SetReadDeadline(_ time.Time) error  { return nil }
func (conn *recordingConn) SetWriteDeadline(_ time.Time) error { return nil }

func (conn *recordingConn) Write(data []byte) (int, error) {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.buf.Write(data)
}

func (conn *recordingConn) Bytes() []byte {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return append([]byte(nil), conn.buf.Bytes()...)
}

type dummyAddr string

func (addr dummyAddr) Network() string { return string(addr) }
func (addr dummyAddr) String() string  { return string(addr) }

func TestSenderQueuesRegisteredConnectionWrites(t *testing.T) {
	oldUpstream, oldDownstream := Upstream, Downstream
	defer func() {
		Upstream, Downstream = oldUpstream, oldDownstream
	}()
	SetUpDownStream("raw", "raw")

	conn := &recordingConn{}
	sender := NewSender(conn)
	defer sender.Close()

	message := NewUpMsg(conn, "secret", "NODE000001")
	header := &Header{
		Sender:      "NODE000001",
		Accepter:    ADMIN_UUID,
		MessageType: MYMEMO,
		RouteLen:    uint32(len(TEMP_ROUTE)),
		Route:       TEMP_ROUTE,
	}

	SendMessage(message, header, &MyMemo{Memo: "queued"}, false)

	if len(conn.Bytes()) == 0 {
		t.Fatal("registered sender did not write queued frame")
	}
}

func TestSenderCloseRejectsFurtherWrites(t *testing.T) {
	conn := &recordingConn{}
	sender := NewSender(conn)
	sender.Close()

	if got := SenderForConn(conn); got != nil {
		t.Fatal("closed sender is still registered for connection")
	}

	err := sender.SendFrame(ControlLane, []byte("frame"))
	if !errors.Is(err, errSenderClosed) {
		t.Fatalf("send after close error = %v, want %v", err, errSenderClosed)
	}
}

func TestSenderOptionsSetLaneQueueCapacity(t *testing.T) {
	conn := &recordingConn{}
	sender := NewSenderWithOptions(conn, SenderOptions{
		ControlQueueSize: 3,
		DataQueueSize:    7,
	})
	defer sender.Close()

	stats := sender.Stats()
	if stats.ControlCapacity != 3 {
		t.Fatalf("control queue capacity = %d, want 3", stats.ControlCapacity)
	}
	if stats.DataCapacity != 7 {
		t.Fatalf("data queue capacity = %d, want 7", stats.DataCapacity)
	}
}

func TestLaneForMessageType(t *testing.T) {
	tests := []struct {
		messageType uint16
		want        Lane
	}{
		{messageType: MYMEMO, want: ControlLane},
		{messageType: FILEDATA, want: DataLane},
		{messageType: SOCKSTCPDATA, want: DataLane},
		{messageType: SOCKSTCPFIN, want: DataLane},
		{messageType: FORWARDDATA, want: DataLane},
		{messageType: FORWARDFIN, want: DataLane},
		{messageType: BACKWARDDATA, want: DataLane},
		{messageType: BACKWARDFIN, want: DataLane},
		{messageType: BACKWARDSTOP, want: ControlLane},
	}

	for _, tc := range tests {
		if got := LaneForMessageType(tc.messageType); got != tc.want {
			t.Fatalf("lane for message type %d = %d, want %d", tc.messageType, got, tc.want)
		}
	}
}
