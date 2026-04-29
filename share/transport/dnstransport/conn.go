package dnstransport

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Conn struct {
	socket    *packetSocket
	sessionID uint64
	peer      *net.UDPAddr
	active    bool

	established chan struct{}
	estOnce     sync.Once
	closeAck    chan struct{}
	closeAckOne sync.Once

	closeCh   chan struct{}
	closeOnce sync.Once
	closeErr  error
	closeMu   sync.Mutex

	lastRxUnixNano atomic.Int64

	readCh chan []byte
	readMu sync.Mutex
	read   bytes.Buffer

	recvMu      sync.Mutex
	recvNextSeq uint64
	recvBuffer  map[uint64]frame
	fragBuffer  map[fragKey]*fragmentBuffer

	sendMu     sync.Mutex
	nextSeq    uint64
	nextFragID uint32
	cwnd       int

	ackMu      sync.Mutex
	ackWaiters map[chan ackEvent]struct{}
	rttMu      sync.Mutex
	srtt       time.Duration
	rttvar     time.Duration
	rto        time.Duration

	outCh chan frame

	deadlineMu    sync.Mutex
	readDeadline  time.Time
	writeDeadline time.Time

	lastPayloadUnixNano atomic.Int64
}

func newConn(socket *packetSocket, sessionID uint64, peer *net.UDPAddr, active bool) *Conn {
	config := socket.config
	conn := &Conn{
		socket:      socket,
		sessionID:   sessionID,
		peer:        cloneUDPAddr(peer),
		active:      active,
		established: make(chan struct{}),
		closeAck:    make(chan struct{}),
		closeCh:     make(chan struct{}),
		readCh:      make(chan []byte, config.RecvQueue),
		recvNextSeq: 1,
		recvBuffer:  make(map[uint64]frame),
		fragBuffer:  make(map[fragKey]*fragmentBuffer),
		nextSeq:     1,
		nextFragID:  1,
		cwnd:        config.InitialWindow,
		ackWaiters:  make(map[chan ackEvent]struct{}),
		rto:         config.RetransmitMin,
		outCh:       make(chan frame, config.SendWindow+config.RecvQueue),
	}
	now := time.Now().UnixNano()
	conn.lastRxUnixNano.Store(now)
	conn.lastPayloadUnixNano.Store(now)
	if !active {
		conn.markEstablished()
	}
	go conn.keepaliveLoop()
	if active {
		go conn.pollLoop()
	}
	return conn
}

func (conn *Conn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		conn.readMu.Lock()
		if conn.read.Len() > 0 {
			n, _ := conn.read.Read(p)
			conn.readMu.Unlock()
			return n, nil
		}
		conn.readMu.Unlock()
		if chunk, ok := conn.popQueuedRead(); ok {
			conn.readMu.Lock()
			_, _ = conn.read.Write(chunk)
			conn.readMu.Unlock()
			continue
		}

		timer, timerCh := conn.readTimer()
		select {
		case chunk := <-conn.readCh:
			if timer != nil {
				timer.Stop()
			}
			if len(chunk) == 0 {
				continue
			}
			conn.readMu.Lock()
			_, _ = conn.read.Write(chunk)
			conn.readMu.Unlock()
		case <-timerCh:
			return 0, timeoutError("DNS read timeout")
		case <-conn.closeCh:
			if timer != nil {
				timer.Stop()
			}
			if chunk, ok := conn.popQueuedRead(); ok {
				conn.readMu.Lock()
				_, _ = conn.read.Write(chunk)
				conn.readMu.Unlock()
				continue
			}
			if err := conn.err(); err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
	}
}

func (conn *Conn) Write(p []byte) (int, error) {
	return conn.WriteWithStream(StreamDefault, p)
}

func (conn *Conn) WriteWithStream(streamID uint32, p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := conn.waitEstablished(); err != nil {
		return 0, err
	}
	conn.touchPayloadActivity()

	conn.sendMu.Lock()
	defer conn.sendMu.Unlock()

	deadline := conn.currentWriteDeadline()
	chunks := splitPayload(p, conn.socket.config.PayloadMTU)
	written := 0
	fragID := conn.nextFragID
	conn.nextFragID++
	if conn.nextFragID == 0 {
		conn.nextFragID = 1
	}
	if len(chunks) > maxUint16 {
		return 0, errors.New("DNS write payload produces too many fragments")
	}
	fragTotal := uint16(len(chunks))
	for start := 0; start < len(chunks); {
		window := conn.currentWindow()
		end := start + window
		if end > len(chunks) {
			end = len(chunks)
		}
		pending := make(map[uint64]frame, end-start)
		for offset, chunk := range chunks[start:end] {
			seq := conn.nextSeq
			conn.nextSeq++
			pending[seq] = frame{
				Type:      frameDATA,
				SessionID: conn.sessionID,
				StreamID:  streamID,
				Seq:       seq,
				FragID:    fragID,
				FragIndex: uint16(start + offset),
				FragTotal: fragTotal,
				Payload:   chunk,
			}
		}
		if err := conn.sendAndWaitACKs(pending, deadline); err != nil {
			conn.onWindowLoss()
			return written, err
		}
		conn.onWindowAcked()
		for _, chunk := range chunks[start:end] {
			written += len(chunk)
		}
		start = end
	}
	return written, nil
}

func (conn *Conn) Close() error {
	var closeErr error
	conn.closeOnce.Do(func() {
		closeErr = conn.closeHandshake()
		close(conn.closeCh)
		if conn.socket != nil {
			conn.socket.unregister(conn.sessionID)
		}
	})
	return closeErr
}

func (conn *Conn) LocalAddr() net.Addr {
	if conn.socket == nil || conn.socket.pc == nil || conn.socket.pc.LocalAddr() == nil {
		return Addr{}
	}
	return Addr{Address: conn.socket.pc.LocalAddr().String() + "/" + conn.socket.domain}
}

func (conn *Conn) RemoteAddr() net.Addr {
	if conn.peer == nil {
		return Addr{}
	}
	return Addr{Address: conn.peer.String() + "/" + conn.socket.domain}
}

func (conn *Conn) SetDeadline(t time.Time) error {
	conn.deadlineMu.Lock()
	conn.readDeadline = t
	conn.writeDeadline = t
	conn.deadlineMu.Unlock()
	return nil
}

func (conn *Conn) SetReadDeadline(t time.Time) error {
	conn.deadlineMu.Lock()
	conn.readDeadline = t
	conn.deadlineMu.Unlock()
	return nil
}

func (conn *Conn) SetWriteDeadline(t time.Time) error {
	conn.deadlineMu.Lock()
	conn.writeDeadline = t
	conn.deadlineMu.Unlock()
	return nil
}

func (conn *Conn) handshake(ctx context.Context) error {
	delay := conn.socket.config.RetransmitMin
	for {
		if err := conn.sendFrameWithContext(ctx, frame{Type: frameSYN, SessionID: conn.sessionID}); err != nil && !isTimeout(err) {
			return err
		}
		select {
		case <-conn.established:
			return nil
		case <-conn.closeCh:
			return conn.err()
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := sleepWithContext(ctx, delay); err != nil {
			return err
		}
		delay *= 2
		if delay > conn.socket.config.RetransmitMax {
			delay = conn.socket.config.RetransmitMax
		}
	}
}

func (conn *Conn) handleFrame(f frame) {
	conn.touchRx()
	if f.Ack > 0 {
		conn.publishACK(ackEvent{Ack: f.Ack})
	}
	switch f.Type {
	case frameSYNACK:
		conn.markEstablished()
	case frameDATA:
		conn.handleDATA(f)
	case frameACK:
	case framePOLL, framePONG:
	case frameCLOSE:
		_ = conn.sendControl(frameCLOSEACK)
		conn.closeWithError(io.EOF)
	case frameRESET:
		conn.closeWithError(net.ErrClosed)
	case frameCLOSEACK:
		conn.markCloseAcked()
	}
}

func (conn *Conn) handleDATA(f frame) {
	conn.touchPayloadActivity()

	conn.recvMu.Lock()
	if f.Seq < conn.recvNextSeq {
		conn.recvMu.Unlock()
		return
	}
	if _, exists := conn.recvBuffer[f.Seq]; !exists && (f.Seq == conn.recvNextSeq || len(conn.recvBuffer) < conn.socket.config.RecvQueue) {
		f.Payload = append([]byte(nil), f.Payload...)
		conn.recvBuffer[f.Seq] = f
	}
	ready := make([][]byte, 0)
	for {
		recvFrame, exists := conn.recvBuffer[conn.recvNextSeq]
		if !exists {
			break
		}
		delete(conn.recvBuffer, conn.recvNextSeq)
		if payload, complete := conn.assembleFrameLocked(recvFrame); complete {
			ready = append(ready, payload)
		}
		conn.recvNextSeq++
	}
	conn.recvMu.Unlock()

	for _, payload := range ready {
		select {
		case conn.readCh <- payload:
		case <-conn.closeCh:
			return
		}
	}
}

func (conn *Conn) nextResponse(wait time.Duration) frame {
	select {
	case f := <-conn.outCh:
		if f.SessionID == 0 {
			f.SessionID = conn.sessionID
		}
		f.Ack = conn.recvAck()
		return f
	default:
	}
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case f := <-conn.outCh:
			if f.SessionID == 0 {
				f.SessionID = conn.sessionID
			}
			f.Ack = conn.recvAck()
			return f
		case <-timer.C:
		case <-conn.closeCh:
		}
	}
	return frame{
		Type:      framePONG,
		SessionID: conn.sessionID,
		Ack:       conn.recvAck(),
	}
}

func (conn *Conn) sendAndWaitACKs(frames map[uint64]frame, deadline time.Time) error {
	if len(frames) == 0 {
		return nil
	}
	acked := make(map[uint64]struct{}, len(frames))
	ackCh := make(chan ackEvent, len(frames)*2)
	conn.registerACKWaiter(ackCh)
	defer conn.unregisterACKWaiter(ackCh)

	sentAt := make(map[uint64]time.Time, len(frames))
	for _, f := range frames {
		if err := conn.sendFrameWithDeadline(f, deadline); err != nil {
			if !isTimeout(err) {
				return err
			}
		}
		sentAt[f.Seq] = time.Now()
	}

	retries := 0
	for len(acked) < len(frames) {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return timeoutError("DNS write timeout")
		}
		wait := conn.currentRTO()
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return timeoutError("DNS write timeout")
			}
			if remaining < wait {
				wait = remaining
			}
		}
		timer := time.NewTimer(wait)
		select {
		case ack := <-ackCh:
			timer.Stop()
			conn.markAcked(frames, sentAt, ack, acked)
		case <-timer.C:
			retries++
			if retries > conn.socket.config.MaxRetries {
				return timeoutError("DNS write ack timeout")
			}
			conn.backoffRTO()
			for seq, f := range frames {
				if _, ok := acked[seq]; ok {
					continue
				}
				if err := conn.sendFrameWithDeadline(f, deadline); err != nil {
					if !isTimeout(err) {
						return err
					}
				}
				sentAt[seq] = time.Now()
			}
		case <-conn.closeCh:
			timer.Stop()
			if err := conn.err(); err != nil {
				return err
			}
			return net.ErrClosed
		}
	}
	return nil
}

func (conn *Conn) sendControl(typ frameType) error {
	return conn.sendFrameWithDeadline(frame{Type: typ, SessionID: conn.sessionID}, conn.currentWriteDeadline())
}

func (conn *Conn) sendFrameWithDeadline(f frame, deadline time.Time) error {
	ctx, cancel := timeoutContext(deadline, conn.socket.config.QueryTimeout)
	defer cancel()
	return conn.sendFrameWithContext(ctx, f)
}

func (conn *Conn) sendFrameWithContext(ctx context.Context, f frame) error {
	select {
	case <-conn.closeCh:
		if err := conn.err(); err != nil {
			return err
		}
		return net.ErrClosed
	default:
	}
	if f.SessionID == 0 {
		f.SessionID = conn.sessionID
	}
	f.Ack = conn.recvAck()
	if conn.active {
		resp, err := conn.socket.exchange(ctx, f)
		if err != nil {
			return wrapDNSTimeout(err, "DNS query timeout")
		}
		conn.handleFrame(resp)
		return nil
	}
	select {
	case conn.outCh <- f:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-conn.closeCh:
		if err := conn.err(); err != nil {
			return err
		}
		return net.ErrClosed
	}
}

func (conn *Conn) markEstablished() {
	conn.estOnce.Do(func() {
		close(conn.established)
	})
}

func (conn *Conn) waitEstablished() error {
	select {
	case <-conn.established:
		return nil
	case <-conn.closeCh:
		if err := conn.err(); err != nil {
			return err
		}
		return net.ErrClosed
	}
}

func (conn *Conn) closeHandshake() error {
	if conn.isClosed() {
		return nil
	}
	deadline := conn.currentWriteDeadline()
	if timeout := conn.socket.config.CloseTimeout; timeout > 0 {
		closeDeadline := time.Now().Add(timeout)
		if deadline.IsZero() || closeDeadline.Before(deadline) {
			deadline = closeDeadline
		}
	}
	_ = conn.sendFrameWithDeadline(frame{Type: frameCLOSE, SessionID: conn.sessionID}, deadline)
	return nil
}

func (conn *Conn) isClosed() bool {
	select {
	case <-conn.closeCh:
		return true
	default:
		return false
	}
}

func (conn *Conn) closeWithError(err error) {
	conn.closeMu.Lock()
	if conn.closeErr == nil && err != nil && !errors.Is(err, io.EOF) {
		conn.closeErr = err
	}
	conn.closeMu.Unlock()
	conn.closeOnce.Do(func() {
		close(conn.closeCh)
		if conn.socket != nil {
			conn.socket.unregister(conn.sessionID)
		}
	})
}

func (conn *Conn) err() error {
	conn.closeMu.Lock()
	defer conn.closeMu.Unlock()
	return conn.closeErr
}

func (conn *Conn) popQueuedRead() ([]byte, bool) {
	select {
	case chunk := <-conn.readCh:
		return chunk, true
	default:
		return nil, false
	}
}

func (conn *Conn) readTimer() (*time.Timer, <-chan time.Time) {
	conn.deadlineMu.Lock()
	deadline := conn.readDeadline
	conn.deadlineMu.Unlock()
	if deadline.IsZero() {
		return nil, nil
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		timer := time.NewTimer(0)
		return timer, timer.C
	}
	timer := time.NewTimer(remaining)
	return timer, timer.C
}

func (conn *Conn) currentWriteDeadline() time.Time {
	conn.deadlineMu.Lock()
	defer conn.deadlineMu.Unlock()
	return conn.writeDeadline
}

func (conn *Conn) recvAck() uint64 {
	conn.recvMu.Lock()
	defer conn.recvMu.Unlock()
	if conn.recvNextSeq == 0 {
		return 0
	}
	return conn.recvNextSeq - 1
}

func (conn *Conn) currentWindow() int {
	if conn.cwnd <= 0 {
		return 1
	}
	if conn.cwnd > conn.socket.config.SendWindow {
		return conn.socket.config.SendWindow
	}
	return conn.cwnd
}

func (conn *Conn) onWindowAcked() {
	if conn.cwnd < conn.socket.config.SendWindow {
		conn.cwnd++
	}
}

func (conn *Conn) onWindowLoss() {
	conn.cwnd /= 2
	if conn.cwnd < 1 {
		conn.cwnd = 1
	}
}

func (conn *Conn) currentRTO() time.Duration {
	conn.rttMu.Lock()
	defer conn.rttMu.Unlock()
	if conn.rto <= 0 {
		return conn.socket.config.RetransmitMin
	}
	return conn.rto
}

func (conn *Conn) observeRTT(sample time.Duration) {
	if sample <= 0 {
		return
	}
	conn.rttMu.Lock()
	defer conn.rttMu.Unlock()
	if conn.srtt == 0 {
		conn.srtt = sample
		conn.rttvar = sample / 2
	} else {
		diff := conn.srtt - sample
		if diff < 0 {
			diff = -diff
		}
		conn.rttvar = (3*conn.rttvar + diff) / 4
		conn.srtt = (7*conn.srtt + sample) / 8
	}
	conn.rto = conn.clampRTO(conn.srtt + 4*conn.rttvar)
}

func (conn *Conn) backoffRTO() {
	conn.rttMu.Lock()
	defer conn.rttMu.Unlock()
	if conn.rto <= 0 {
		conn.rto = conn.socket.config.RetransmitMin
	}
	conn.rto = conn.clampRTO(conn.rto * 2)
}

func (conn *Conn) clampRTO(value time.Duration) time.Duration {
	if value < conn.socket.config.RetransmitMin {
		return conn.socket.config.RetransmitMin
	}
	if value > conn.socket.config.RetransmitMax {
		return conn.socket.config.RetransmitMax
	}
	return value
}

func (conn *Conn) markAcked(frames map[uint64]frame, sentAt map[uint64]time.Time, ack ackEvent, acked map[uint64]struct{}) {
	now := time.Now()
	for seq := range frames {
		if seq <= ack.Ack {
			if _, exists := acked[seq]; !exists {
				acked[seq] = struct{}{}
				if sent := sentAt[seq]; !sent.IsZero() {
					conn.observeRTT(now.Sub(sent))
				}
			}
		}
	}
	for _, seq := range ack.Selective {
		if _, exists := frames[seq]; !exists {
			continue
		}
		if _, exists := acked[seq]; exists {
			continue
		}
		acked[seq] = struct{}{}
		if sent := sentAt[seq]; !sent.IsZero() {
			conn.observeRTT(now.Sub(sent))
		}
	}
}

func (conn *Conn) assembleFrameLocked(f frame) ([]byte, bool) {
	if f.FragTotal <= 1 {
		return append([]byte(nil), f.Payload...), true
	}
	if f.FragIndex >= f.FragTotal || int(f.FragTotal) > conn.socket.config.RecvQueue {
		return nil, false
	}
	key := fragKey{StreamID: f.StreamID, FragID: f.FragID}
	buf := conn.fragBuffer[key]
	if buf == nil {
		if len(conn.fragBuffer) >= conn.socket.config.RecvQueue {
			return nil, false
		}
		buf = newFragmentBuffer(f.FragTotal)
		conn.fragBuffer[key] = buf
	}
	buf.add(f.FragIndex, f.Payload)
	if !buf.complete() {
		return nil, false
	}
	delete(conn.fragBuffer, key)
	return buf.bytes(), true
}

func (conn *Conn) keepaliveLoop() {
	if conn.socket == nil {
		return
	}
	idleTimeout := conn.socket.config.IdleTimeout
	if idleTimeout <= 0 {
		return
	}
	interval := idleTimeout / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			idleFor := time.Since(time.Unix(0, conn.lastRxUnixNano.Load()))
			if idleFor > idleTimeout {
				conn.closeWithError(timeoutError("DNS session idle timeout"))
				return
			}
		case <-conn.closeCh:
			return
		}
	}
}

func (conn *Conn) pollLoop() {
	if !conn.active {
		return
	}
	if err := conn.waitEstablished(); err != nil {
		return
	}
	misses := 0
	for {
		interval := conn.currentPollInterval(time.Now())
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
		case <-conn.closeCh:
			timer.Stop()
			return
		}
		err := conn.sendFrameWithDeadline(frame{Type: framePOLL, SessionID: conn.sessionID}, time.Time{})
		if err != nil {
			misses++
			if misses > conn.socket.config.MaxRetries {
				conn.closeWithError(timeoutError("DNS poll timeout"))
				return
			}
			continue
		}
		misses = 0
	}
}

func (conn *Conn) currentPollInterval(now time.Time) time.Duration {
	interval := conn.socket.config.PollInterval
	if lastPayload := conn.lastPayloadUnixNano.Load(); lastPayload > 0 {
		idleFor := now.Sub(time.Unix(0, lastPayload))
		if idleFor > conn.socket.config.IdlePollInterval {
			interval = conn.socket.config.IdlePollInterval
		}
	}
	return interval
}

func (conn *Conn) touchRx() {
	conn.lastRxUnixNano.Store(time.Now().UnixNano())
}

func (conn *Conn) touchPayloadActivity() {
	conn.lastPayloadUnixNano.Store(time.Now().UnixNano())
}

func (conn *Conn) markCloseAcked() {
	conn.closeAckOne.Do(func() {
		close(conn.closeAck)
	})
}

func (conn *Conn) registerACKWaiter(ch chan ackEvent) {
	conn.ackMu.Lock()
	conn.ackWaiters[ch] = struct{}{}
	conn.ackMu.Unlock()
}

func (conn *Conn) unregisterACKWaiter(ch chan ackEvent) {
	conn.ackMu.Lock()
	delete(conn.ackWaiters, ch)
	conn.ackMu.Unlock()
}

func (conn *Conn) publishACK(ack ackEvent) {
	conn.ackMu.Lock()
	waiters := make([]chan ackEvent, 0, len(conn.ackWaiters))
	for ch := range conn.ackWaiters {
		waiters = append(waiters, ch)
	}
	conn.ackMu.Unlock()
	for _, ch := range waiters {
		select {
		case ch <- ack:
		default:
		}
	}
}

func splitPayload(p []byte, mtu int) [][]byte {
	if mtu <= 0 || mtu > len(p) {
		mtu = len(p)
	}
	chunks := make([][]byte, 0, (len(p)+mtu-1)/mtu)
	for len(p) > 0 {
		n := mtu
		if len(p) < n {
			n = len(p)
		}
		chunks = append(chunks, append([]byte(nil), p[:n]...))
		p = p[n:]
	}
	return chunks
}

type ackEvent struct {
	Ack       uint64
	Selective []uint64
}

type fragKey struct {
	StreamID uint32
	FragID   uint32
}

type fragmentBuffer struct {
	parts [][]byte
	seen  []bool
	count int
}

func newFragmentBuffer(total uint16) *fragmentBuffer {
	return &fragmentBuffer{
		parts: make([][]byte, int(total)),
		seen:  make([]bool, int(total)),
	}
}

func (buf *fragmentBuffer) add(index uint16, payload []byte) {
	if int(index) >= len(buf.parts) {
		return
	}
	if buf.seen[index] {
		return
	}
	buf.parts[index] = append([]byte(nil), payload...)
	buf.seen[index] = true
	buf.count++
}

func (buf *fragmentBuffer) complete() bool {
	return buf.count == len(buf.parts)
}

func (buf *fragmentBuffer) bytes() []byte {
	size := 0
	for _, part := range buf.parts {
		size += len(part)
	}
	out := make([]byte, 0, size)
	for _, part := range buf.parts {
		out = append(out, part...)
	}
	return out
}

func encodeSelectiveACK(seqs []uint64) []byte {
	if len(seqs) == 0 {
		return nil
	}
	if len(seqs) > maxUint16 {
		seqs = seqs[:maxUint16]
	}
	out := make([]byte, 2+len(seqs)*8)
	binary.BigEndian.PutUint16(out[:2], uint16(len(seqs)))
	offset := 2
	for _, seq := range seqs {
		binary.BigEndian.PutUint64(out[offset:offset+8], seq)
		offset += 8
	}
	return out
}

func decodeSelectiveACK(payload []byte) []uint64 {
	if len(payload) < 2 {
		return nil
	}
	count := int(binary.BigEndian.Uint16(payload[:2]))
	if count <= 0 {
		return nil
	}
	if len(payload) < 2+count*8 {
		return nil
	}
	seqs := make([]uint64, 0, count)
	offset := 2
	for i := 0; i < count; i++ {
		seqs = append(seqs, binary.BigEndian.Uint64(payload[offset:offset+8]))
		offset += 8
	}
	return seqs
}

func cloneUDPAddr(addr *net.UDPAddr) *net.UDPAddr {
	if addr == nil {
		return nil
	}
	out := *addr
	if addr.IP != nil {
		out.IP = append(net.IP(nil), addr.IP...)
	}
	return &out
}
