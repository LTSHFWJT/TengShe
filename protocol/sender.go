package protocol

import (
	"errors"
	"io"
	"net"
	"sync"
)

type Lane int

const (
	ControlLane Lane = iota
	DataLane
)

const (
	defaultControlQueueSize = 64
	defaultDataQueueSize    = 256
)

var errSenderClosed = errors.New("sender is closed")

var senderRegistry sync.Map

type SenderOptions struct {
	ControlQueueSize int
	DataQueueSize    int
}

type SenderStats struct {
	ControlQueued   int
	ControlCapacity int
	DataQueued      int
	DataCapacity    int
}

type Sender struct {
	conn     net.Conn
	control  chan outboundFrame
	data     chan outboundFrame
	closed   chan struct{}
	done     chan struct{}
	once     sync.Once
	mu       sync.RWMutex
	isClosed bool
}

type outboundFrame struct {
	data []byte
	done chan error
}

func NewSender(conn net.Conn) *Sender {
	return NewSenderWithOptions(conn, SenderOptions{})
}

func NewSenderWithOptions(conn net.Conn, options SenderOptions) *Sender {
	if options.ControlQueueSize <= 0 {
		options.ControlQueueSize = defaultControlQueueSize
	}
	if options.DataQueueSize <= 0 {
		options.DataQueueSize = defaultDataQueueSize
	}

	sender := &Sender{
		conn:    conn,
		control: make(chan outboundFrame, options.ControlQueueSize),
		data:    make(chan outboundFrame, options.DataQueueSize),
		closed:  make(chan struct{}),
		done:    make(chan struct{}),
	}
	senderRegistry.Store(conn, sender)
	go sender.run()
	return sender
}

func SenderForConn(conn net.Conn) *Sender {
	if conn == nil {
		return nil
	}

	sender, ok := senderRegistry.Load(conn)
	if !ok {
		return nil
	}

	return sender.(*Sender)
}

func (sender *Sender) SendMessage(lane Lane, message Message, header *Header, mess interface{}, isPass bool) error {
	frame, err := MarshalMessage(message, header, mess, isPass)
	if err != nil {
		return err
	}
	return sender.SendFrame(lane, frame)
}

func (sender *Sender) SendFrame(lane Lane, frame []byte) error {
	if sender == nil {
		return errSenderClosed
	}

	sender.mu.RLock()
	defer sender.mu.RUnlock()
	if sender.isClosed {
		return errSenderClosed
	}

	item := outboundFrame{
		data: frame,
		done: make(chan error, 1),
	}

	queue := sender.control
	if lane == DataLane {
		queue = sender.data
	}

	select {
	case <-sender.closed:
		return errSenderClosed
	case queue <- item:
	}

	return <-item.done
}

func (sender *Sender) Stats() SenderStats {
	if sender == nil {
		return SenderStats{}
	}

	return SenderStats{
		ControlQueued:   len(sender.control),
		ControlCapacity: cap(sender.control),
		DataQueued:      len(sender.data),
		DataCapacity:    cap(sender.data),
	}
}

func (sender *Sender) Close() {
	if sender == nil {
		return
	}

	sender.once.Do(func() {
		sender.mu.Lock()
		sender.isClosed = true
		close(sender.closed)
		sender.mu.Unlock()
		<-sender.done
	})
}

func (sender *Sender) run() {
	defer func() {
		if current, ok := senderRegistry.Load(sender.conn); ok && current == sender {
			senderRegistry.Delete(sender.conn)
		}
		sender.failPending()
		close(sender.done)
	}()

	for {
		select {
		case item := <-sender.control:
			item.done <- writeFullLane(sender.conn, ControlLane, item.data)
			continue
		default:
		}

		select {
		case item := <-sender.control:
			item.done <- writeFullLane(sender.conn, ControlLane, item.data)
		case item := <-sender.data:
			item.done <- writeFullLane(sender.conn, DataLane, item.data)
		case <-sender.closed:
			return
		}
	}
}

func (sender *Sender) failPending() {
	for {
		select {
		case item := <-sender.control:
			item.done <- errSenderClosed
		case item := <-sender.data:
			item.done <- errSenderClosed
		default:
			return
		}
	}
}

func MarshalMessage(message Message, header *Header, mess interface{}, isPass bool) ([]byte, error) {
	if message == nil {
		return nil, errors.New("nil protocol message")
	}

	ConstructMessage(message, header, mess, isPass)

	switch msg := message.(type) {
	case *RawMessage:
		return drainRawFrame(msg), nil
	case *HTTPMessage:
		return drainHTTPFrame(msg), nil
	case *WSMessage:
		return drainRawFrame(msg.RawMessage), nil
	default:
		return nil, errors.New("unsupported protocol message type")
	}
}

func LaneForMessageType(messageType uint16) Lane {
	if IsDataMessageType(messageType) {
		return DataLane
	}
	return ControlLane
}

func IsDataMessageType(messageType uint16) bool {
	switch messageType {
	case FILEDATA, SOCKSTCPDATA, SOCKSUDPDATA, SOCKSTCPFIN, FORWARDDATA, FORWARDFIN, BACKWARDDATA, BACKWARDFIN:
		return true
	default:
		return false
	}
}

func writeFull(conn net.Conn, data []byte) error {
	return writeFullLane(conn, ControlLane, data)
}

type streamWriter interface {
	WriteWithStream(streamID uint32, p []byte) (int, error)
}

func writeFullLane(conn net.Conn, lane Lane, data []byte) error {
	if writer, ok := conn.(streamWriter); ok {
		streamID := uint32(1)
		if lane == DataLane {
			streamID = 2
		}
		return writeFullWithStream(writer, streamID, data)
	}
	return writeFullPlain(conn, data)
}

func writeFullWithStream(conn streamWriter, streamID uint32, data []byte) error {
	for len(data) > 0 {
		n, err := conn.WriteWithStream(streamID, data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func writeFullPlain(conn net.Conn, data []byte) error {
	for len(data) > 0 {
		n, err := conn.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
