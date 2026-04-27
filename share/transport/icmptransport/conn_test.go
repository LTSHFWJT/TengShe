package icmptransport

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/ipv4"
)

type sentFrame struct {
	typ   ipv4.ICMPType
	frame frame
}

func newTestConn(t *testing.T, config Config) (*Conn, *[]sentFrame, *sync.Mutex) {
	t.Helper()
	config = normalizeConfig(config)
	socket := &packetSocket{
		config:     config,
		endpointID: 1,
		accepting:  true,
		conns:      make(map[uint64]*Conn),
		acceptCh:   make(chan *Conn, config.AcceptQueue),
	}
	conn := newConn(socket, 2, &net.IPAddr{IP: net.IPv4(127, 0, 0, 1)}, false)
	sent := make([]sentFrame, 0)
	var mu sync.Mutex
	conn.writePacket = func(_ *net.IPAddr, typ ipv4.ICMPType, f frame) error {
		mu.Lock()
		sent = append(sent, sentFrame{typ: typ, frame: f})
		mu.Unlock()
		return nil
	}
	t.Cleanup(func() {
		conn.closeWithError(io.EOF)
	})
	return conn, &sent, &mu
}

func TestConnReordersDataFrames(t *testing.T) {
	conn, sent, mu := newTestConn(t, Config{IdleTimeout: time.Hour})
	conn.SetReadDeadline(time.Now().Add(time.Second))

	conn.handleFrame(frame{Type: frameDATA, SessionID: conn.sessionID, Seq: 2, Payload: []byte("bb")})
	conn.handleFrame(frame{Type: frameDATA, SessionID: conn.sessionID, Seq: 1, Payload: []byte("aa")})

	buf := make([]byte, 2)
	n, err := io.ReadFull(conn, buf)
	if err != nil {
		t.Fatalf("first read error after %d bytes: %v", n, err)
	}
	if string(buf) != "aa" {
		t.Fatalf("first read = %q, want aa", buf)
	}
	n, err = io.ReadFull(conn, buf)
	if err != nil {
		t.Fatalf("second read error after %d bytes: %v", n, err)
	}
	if string(buf) != "bb" {
		t.Fatalf("second read = %q, want bb", buf)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(*sent) != 2 {
		t.Fatalf("ACK count = %d, want 2", len(*sent))
	}
	if (*sent)[0].frame.Type != frameACK || (*sent)[0].frame.Ack != 0 {
		t.Fatalf("first ACK = %+v, want ACK 0 with SACK", (*sent)[0].frame)
	}
	if got := decodeSelectiveACK((*sent)[0].frame.Payload); len(got) != 1 || got[0] != 2 {
		t.Fatalf("first ACK SACK = %v, want [2]", got)
	}
	if (*sent)[1].frame.Type != frameACK || (*sent)[1].frame.Ack != 2 {
		t.Fatalf("second ACK = %+v, want cumulative ACK 2", (*sent)[1].frame)
	}
}

func TestConnWriteWaitsForPerConnACK(t *testing.T) {
	conn, sent, mu := newTestConn(t, Config{
		PayloadMTU:       2,
		SendWindow:       2,
		RetransmitMin:    10 * time.Millisecond,
		RetransmitMax:    10 * time.Millisecond,
		MaxRetries:       2,
		IdleTimeout:      time.Hour,
		HandshakeTimeout: time.Second,
	})
	conn.writePacket = func(_ *net.IPAddr, typ ipv4.ICMPType, f frame) error {
		mu.Lock()
		*sent = append(*sent, sentFrame{typ: typ, frame: f})
		mu.Unlock()
		if f.Type == frameDATA {
			go conn.handleFrame(frame{Type: frameACK, SessionID: conn.sessionID, Ack: f.Seq})
		}
		return nil
	}

	n, err := conn.Write([]byte("abcd"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 4 {
		t.Fatalf("Write bytes = %d, want 4", n)
	}

	mu.Lock()
	defer mu.Unlock()
	dataFrames := 0
	for _, each := range *sent {
		if each.frame.Type == frameDATA {
			dataFrames++
		}
	}
	if dataFrames != 2 {
		t.Fatalf("DATA frames = %d, want 2", dataFrames)
	}
}

func TestConnWriteTimesOutWithoutACK(t *testing.T) {
	conn, _, _ := newTestConn(t, Config{
		PayloadMTU:    2,
		SendWindow:    1,
		RetransmitMin: 2 * time.Millisecond,
		RetransmitMax: 2 * time.Millisecond,
		MaxRetries:    1,
		IdleTimeout:   time.Hour,
	})

	_, err := conn.Write([]byte("ab"))
	if err == nil {
		t.Fatal("Write succeeded without ACK")
	}
	if timeout, ok := err.(interface{ Timeout() bool }); !ok || !timeout.Timeout() {
		t.Fatalf("Write error = %T %v, want timeout", err, err)
	}
}

func TestConnWriteReturnsConfirmedPartialCount(t *testing.T) {
	conn, sent, mu := newTestConn(t, Config{
		PayloadMTU:    2,
		SendWindow:    1,
		RetransmitMin: 2 * time.Millisecond,
		RetransmitMax: 2 * time.Millisecond,
		MaxRetries:    1,
		IdleTimeout:   time.Hour,
	})
	conn.writePacket = func(_ *net.IPAddr, typ ipv4.ICMPType, f frame) error {
		mu.Lock()
		*sent = append(*sent, sentFrame{typ: typ, frame: f})
		mu.Unlock()
		if f.Type == frameDATA && f.Seq == 1 {
			go conn.handleFrame(frame{Type: frameACK, SessionID: conn.sessionID, Ack: 1})
		}
		return nil
	}

	n, err := conn.Write([]byte("abcd"))
	if err == nil {
		t.Fatal("Write unexpectedly succeeded")
	}
	if n != 2 {
		t.Fatalf("partial Write bytes = %d, want 2", n)
	}
}

func TestConnCumulativeACKConfirmsWindow(t *testing.T) {
	conn, sent, mu := newTestConn(t, Config{
		PayloadMTU:    2,
		SendWindow:    2,
		RetransmitMin: 5 * time.Millisecond,
		RetransmitMax: 5 * time.Millisecond,
		MaxRetries:    1,
		IdleTimeout:   time.Hour,
	})
	conn.writePacket = func(_ *net.IPAddr, typ ipv4.ICMPType, f frame) error {
		mu.Lock()
		*sent = append(*sent, sentFrame{typ: typ, frame: f})
		mu.Unlock()
		if f.Type == frameDATA && f.Seq == 2 {
			go conn.handleFrame(frame{Type: frameACK, SessionID: conn.sessionID, Ack: 2})
		}
		return nil
	}

	n, err := conn.Write([]byte("abcd"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 4 {
		t.Fatalf("Write bytes = %d, want 4", n)
	}
}

func TestConnSelectiveACKAvoidsRetransmittingAckedFrame(t *testing.T) {
	conn, sent, mu := newTestConn(t, Config{
		PayloadMTU:    2,
		SendWindow:    2,
		InitialWindow: 2,
		RetransmitMin: 2 * time.Millisecond,
		RetransmitMax: 2 * time.Millisecond,
		MaxRetries:    3,
		IdleTimeout:   time.Hour,
	})
	var retransmittedSeq1 bool
	conn.writePacket = func(_ *net.IPAddr, typ ipv4.ICMPType, f frame) error {
		mu.Lock()
		*sent = append(*sent, sentFrame{typ: typ, frame: f})
		seq1Count := 0
		for _, each := range *sent {
			if each.frame.Type == frameDATA && each.frame.Seq == 1 {
				seq1Count++
			}
		}
		if seq1Count > 1 {
			retransmittedSeq1 = true
		}
		mu.Unlock()

		if f.Type == frameDATA && f.Seq == 2 {
			go conn.handleFrame(frame{Type: frameACK, SessionID: conn.sessionID, Ack: 0, Payload: encodeSelectiveACK([]uint64{2})})
		}
		if f.Type == frameDATA && f.Seq == 1 && retransmittedSeq1 {
			go conn.handleFrame(frame{Type: frameACK, SessionID: conn.sessionID, Ack: 2})
		}
		return nil
	}

	n, err := conn.Write([]byte("abcd"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 4 {
		t.Fatalf("Write bytes = %d, want 4", n)
	}

	mu.Lock()
	defer mu.Unlock()
	seq2Count := 0
	for _, each := range *sent {
		if each.frame.Type == frameDATA && each.frame.Seq == 2 {
			seq2Count++
		}
	}
	if seq2Count != 1 {
		t.Fatalf("seq2 sends = %d, want 1", seq2Count)
	}
}

func TestConnWriteWithStreamSetsFrameStreamID(t *testing.T) {
	conn, sent, mu := newTestConn(t, Config{
		PayloadMTU:    8,
		SendWindow:    1,
		RetransmitMin: 5 * time.Millisecond,
		RetransmitMax: 5 * time.Millisecond,
		MaxRetries:    1,
		IdleTimeout:   time.Hour,
	})
	conn.writePacket = func(_ *net.IPAddr, typ ipv4.ICMPType, f frame) error {
		mu.Lock()
		*sent = append(*sent, sentFrame{typ: typ, frame: f})
		mu.Unlock()
		if f.Type == frameDATA {
			go conn.handleFrame(frame{Type: frameACK, SessionID: conn.sessionID, Ack: f.Seq})
		}
		return nil
	}

	if _, err := conn.WriteWithStream(StreamData, []byte("abc")); err != nil {
		t.Fatalf("WriteWithStream error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, each := range *sent {
		if each.frame.Type == frameDATA && each.frame.StreamID != StreamData {
			t.Fatalf("DATA StreamID = %d, want %d", each.frame.StreamID, StreamData)
		}
	}
}

func TestConnCloseWaitsForCloseACK(t *testing.T) {
	conn, sent, mu := newTestConn(t, Config{
		CloseTimeout:  50 * time.Millisecond,
		RetransmitMin: 5 * time.Millisecond,
		RetransmitMax: 5 * time.Millisecond,
		MaxRetries:    2,
		IdleTimeout:   time.Hour,
	})
	conn.writePacket = func(_ *net.IPAddr, typ ipv4.ICMPType, f frame) error {
		mu.Lock()
		*sent = append(*sent, sentFrame{typ: typ, frame: f})
		mu.Unlock()
		if f.Type == frameCLOSE {
			go conn.handleFrame(frame{Type: frameCLOSEACK, SessionID: conn.sessionID})
		}
		return nil
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*sent) == 0 || (*sent)[0].frame.Type != frameCLOSE {
		t.Fatalf("first sent frame = %+v, want CLOSE", *sent)
	}
}

func TestConnRemoteCloseDrainsQueuedData(t *testing.T) {
	conn, _, _ := newTestConn(t, Config{IdleTimeout: time.Hour})
	conn.handleFrame(frame{Type: frameDATA, SessionID: conn.sessionID, Seq: 1, Payload: []byte("aa")})
	conn.handleFrame(frame{Type: frameCLOSE, SessionID: conn.sessionID})

	buf := make([]byte, 2)
	n, err := io.ReadFull(conn, buf)
	if err != nil {
		t.Fatalf("ReadFull error after %d bytes: %v", n, err)
	}
	if string(buf) != "aa" {
		t.Fatalf("read = %q, want aa", buf)
	}
	n, err = conn.Read(buf)
	if err != io.EOF {
		t.Fatalf("Read after drain = n:%d err:%v, want EOF", n, err)
	}
}

func TestConnCloseTimesOutWithoutCloseACK(t *testing.T) {
	conn, _, _ := newTestConn(t, Config{
		CloseTimeout:  3 * time.Millisecond,
		RetransmitMin: 2 * time.Millisecond,
		RetransmitMax: 2 * time.Millisecond,
		MaxRetries:    10,
		IdleTimeout:   time.Hour,
	})

	err := conn.Close()
	if err == nil {
		t.Fatal("Close succeeded without CLOSEACK")
	}
	if timeout, ok := err.(interface{ Timeout() bool }); !ok || !timeout.Timeout() {
		t.Fatalf("Close error = %T %v, want timeout", err, err)
	}
}

func TestConnRTOAndWindowAdapt(t *testing.T) {
	conn, _, _ := newTestConn(t, Config{
		SendWindow:    8,
		InitialWindow: 2,
		RetransmitMin: 10 * time.Millisecond,
		RetransmitMax: time.Second,
		IdleTimeout:   time.Hour,
	})

	if got := conn.currentWindow(); got != 2 {
		t.Fatalf("initial window = %d, want 2", got)
	}
	conn.onWindowAcked()
	if got := conn.currentWindow(); got != 3 {
		t.Fatalf("acked window = %d, want 3", got)
	}
	conn.onWindowLoss()
	if got := conn.currentWindow(); got != 1 {
		t.Fatalf("loss window = %d, want 1", got)
	}

	conn.observeRTT(80 * time.Millisecond)
	if got := conn.currentRTO(); got <= 10*time.Millisecond {
		t.Fatalf("RTO = %s, want greater than min", got)
	}
	conn.backoffRTO()
	if got := conn.currentRTO(); got <= 80*time.Millisecond {
		t.Fatalf("backed off RTO = %s, want growth", got)
	}
}
