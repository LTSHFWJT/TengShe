package icmptransport

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type packetSocket struct {
	pc         *icmp.PacketConn
	config     Config
	endpointID uint64
	accepting  bool
	datagram   bool

	mu       sync.Mutex
	conns    map[uint64]*Conn
	acceptCh chan *Conn
	closed   bool
}

func newPacketSocket(bind string, config Config, accepting bool) (*packetSocket, error) {
	bind, err := NormalizeListenAddress(bind)
	if err != nil {
		return nil, err
	}
	pc, datagram, err := listenICMPPacket(bind, accepting)
	if err != nil {
		return nil, err
	}
	socket := &packetSocket{
		pc:         pc,
		config:     normalizeConfig(config),
		endpointID: randomID(),
		accepting:  accepting,
		datagram:   datagram,
		conns:      make(map[uint64]*Conn),
		acceptCh:   make(chan *Conn, normalizeConfig(config).AcceptQueue),
	}
	go socket.readLoop()
	return socket, nil
}

func (socket *packetSocket) readLoop() {
	buf := make([]byte, 64*1024)
	for {
		n, src, err := socket.pc.ReadFrom(buf)
		if err != nil {
			socket.closeAll(err)
			return
		}
		msg, err := icmp.ParseMessage(1, buf[:n])
		if err != nil {
			continue
		}
		echo, ok := msg.Body.(*icmp.Echo)
		if !ok || len(echo.Data) == 0 {
			continue
		}
		f, err := decodeFrame(echo.Data)
		if err != nil {
			continue
		}
		if f.SenderID == socket.endpointID {
			continue
		}
		peer := ipAddrFromNetAddr(src)
		if f.Type == frameSYN && socket.accepting {
			socket.handleSYN(peer, f)
			continue
		}
		socket.mu.Lock()
		conn := socket.conns[f.SessionID]
		socket.mu.Unlock()
		if conn == nil {
			continue
		}
		if !sameIPAddr(conn.peer, peer) {
			continue
		}
		conn.handleFrame(f)
	}
}

func (socket *packetSocket) handleSYN(peer *net.IPAddr, f frame) {
	socket.mu.Lock()
	if socket.closed {
		socket.mu.Unlock()
		return
	}
	conn := socket.conns[f.SessionID]
	if conn == nil {
		conn = newConn(socket, f.SessionID, peer, false)
		socket.conns[f.SessionID] = conn
		select {
		case socket.acceptCh <- conn:
		default:
			delete(socket.conns, f.SessionID)
			socket.mu.Unlock()
			conn.closeWithError(errors.New("ICMP accept queue full"))
			return
		}
	}
	socket.mu.Unlock()
	_ = conn.sendControl(frameSYNACK)
}

func (socket *packetSocket) register(conn *Conn) error {
	socket.mu.Lock()
	defer socket.mu.Unlock()
	if socket.closed {
		return net.ErrClosed
	}
	socket.conns[conn.sessionID] = conn
	return nil
}

func (socket *packetSocket) unregister(sessionID uint64) {
	socket.mu.Lock()
	delete(socket.conns, sessionID)
	shouldClose := socket.closed || !socket.accepting
	empty := len(socket.conns) == 0
	socket.mu.Unlock()
	if shouldClose && empty && socket.pc != nil {
		_ = socket.pc.Close()
	}
}

func (socket *packetSocket) closeAccepting() error {
	socket.mu.Lock()
	if socket.closed {
		socket.mu.Unlock()
		return net.ErrClosed
	}
	socket.closed = true
	close(socket.acceptCh)
	empty := len(socket.conns) == 0
	socket.mu.Unlock()
	if empty {
		return socket.pc.Close()
	}
	return nil
}

func (socket *packetSocket) closeAll(err error) {
	socket.mu.Lock()
	if socket.closed {
		socket.mu.Unlock()
		return
	}
	socket.closed = true
	conns := make([]*Conn, 0, len(socket.conns))
	for _, conn := range socket.conns {
		conns = append(conns, conn)
	}
	socket.conns = make(map[uint64]*Conn)
	close(socket.acceptCh)
	socket.mu.Unlock()
	for _, conn := range conns {
		conn.closeWithError(err)
	}
}

func (socket *packetSocket) writeFrame(peer *net.IPAddr, typ ipv4.ICMPType, f frame) error {
	f.SenderID = socket.endpointID
	payload, err := encodeFrame(f)
	if err != nil {
		return err
	}
	echo := &icmp.Echo{
		ID:   int(f.SessionID & 0xffff),
		Seq:  int(f.Seq & 0xffff),
		Data: payload,
	}
	msg := &icmp.Message{
		Type: typ,
		Code: 0,
		Body: echo,
	}
	raw, err := msg.Marshal(nil)
	if err != nil {
		return err
	}
	addr := net.Addr(peer)
	if socket.datagram {
		addr = &net.UDPAddr{IP: peer.IP, Zone: peer.Zone}
	}
	_, err = socket.pc.WriteTo(raw, addr)
	return err
}

func listenICMPPacket(bind string, accepting bool) (*icmp.PacketConn, bool, error) {
	rawConn, rawErr := icmp.ListenPacket("ip4:icmp", bind)
	if rawErr == nil {
		return rawConn, false, nil
	}

	if accepting {
		return nil, false, fmt.Errorf(
			"listen ICMP failed on %s: passive ICMP listener requires a raw endpoint: %v. "+
				"Run the listening node with sudo/root or grant CAP_NET_RAW",
			bind,
			rawErr,
		)
	}

	dgramConn, dgramErr := icmp.ListenPacket("udp4", bind)
	if dgramErr == nil {
		return dgramConn, true, nil
	}

	return nil, false, fmt.Errorf(
		"listen ICMP failed on %s: raw endpoint error: %v; datagram endpoint error: %v. "+
			"Run with sudo/root or CAP_NET_RAW, or ensure the OS allows non-privileged udp4 ICMP endpoints",
		bind,
		rawErr,
		dgramErr,
	)
}

func ipAddrFromNetAddr(addr net.Addr) *net.IPAddr {
	switch v := addr.(type) {
	case *net.IPAddr:
		return &net.IPAddr{IP: append(net.IP(nil), v.IP...), Zone: v.Zone}
	case *net.UDPAddr:
		return &net.IPAddr{IP: append(net.IP(nil), v.IP...), Zone: v.Zone}
	default:
		return &net.IPAddr{IP: net.IPv4zero}
	}
}

func sameIPAddr(left, right *net.IPAddr) bool {
	if left == nil || right == nil {
		return false
	}
	return left.IP.Equal(right.IP)
}

func resolvePeer(value string) (*net.IPAddr, error) {
	host, err := NormalizePeerAddress(value)
	if err != nil {
		return nil, err
	}
	peer, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return nil, err
	}
	if peer == nil || peer.IP == nil || peer.IP.To4() == nil {
		return nil, fmt.Errorf("ICMP transport currently supports IPv4 only: %q", value)
	}
	return peer, nil
}

func randomID() uint64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 1
	}
	id := binary.BigEndian.Uint64(b[:])
	if id == 0 {
		return 1
	}
	return id
}
