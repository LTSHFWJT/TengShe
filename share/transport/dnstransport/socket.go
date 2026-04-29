package dnstransport

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type packetSocket struct {
	pc         *net.UDPConn
	config     Config
	endpointID uint64
	domain     string
	resolver   *net.UDPAddr
	accepting  bool

	packetID  atomic.Uint64
	exchangeM sync.Mutex

	mu       sync.Mutex
	conns    map[uint64]*Conn
	acceptCh chan *Conn
	closed   bool

	cacheMu    sync.Mutex
	cache      map[cacheKey]frame
	cacheOrder []cacheKey
}

func newListenSocket(address string, config Config) (*packetSocket, error) {
	spec, err := parseListenSpec(address)
	if err != nil {
		return nil, err
	}
	udpAddr, err := net.ResolveUDPAddr("udp", spec.Bind)
	if err != nil {
		return nil, err
	}
	pc, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}
	config = normalizeConfig(config)
	config, err = configForDomain(spec.Domain, config)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	socket := &packetSocket{
		pc:         pc,
		config:     config,
		endpointID: randomID(),
		domain:     spec.Domain,
		accepting:  true,
		conns:      make(map[uint64]*Conn),
		acceptCh:   make(chan *Conn, config.AcceptQueue),
		cache:      make(map[cacheKey]frame),
	}
	go socket.readLoop()
	return socket, nil
}

func newDialSocket(address string, bind string, config Config) (*packetSocket, error) {
	spec, err := parseDialSpec(address)
	if err != nil {
		return nil, err
	}
	resolver, err := net.ResolveUDPAddr("udp", spec.Resolver)
	if err != nil {
		return nil, err
	}
	var local *net.UDPAddr
	if strings.TrimSpace(bind) != "" {
		normalized, err := normalizeBind(bind)
		if err != nil {
			return nil, err
		}
		local, err = net.ResolveUDPAddr("udp", normalized)
		if err != nil {
			return nil, err
		}
	}
	pc, err := net.ListenUDP("udp", local)
	if err != nil {
		return nil, err
	}
	config = normalizeConfig(config)
	config, err = configForDomain(spec.Domain, config)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	return &packetSocket{
		pc:         pc,
		config:     config,
		endpointID: randomID(),
		domain:     spec.Domain,
		resolver:   resolver,
		accepting:  false,
		conns:      make(map[uint64]*Conn),
		acceptCh:   make(chan *Conn, config.AcceptQueue),
		cache:      make(map[cacheKey]frame),
	}, nil
}

func (socket *packetSocket) readLoop() {
	buf := make([]byte, 64*1024)
	for {
		n, addr, err := socket.pc.ReadFromUDP(buf)
		if err != nil {
			socket.closeAll(err)
			return
		}
		query, err := parseQuery(buf[:n], socket.domain)
		if err != nil {
			if basic, basicErr := parseBasicQuery(buf[:n]); basicErr == nil {
				if raw, nxErr := buildNXDOMAIN(basic); nxErr == nil {
					_, _ = socket.pc.WriteToUDP(raw, addr)
				}
			}
			continue
		}
		f, err := decodeFrame(query.Payload)
		if err != nil {
			continue
		}
		if f.SenderID == socket.endpointID {
			continue
		}
		key := cacheKey{SessionID: f.SessionID, SenderID: f.SenderID, PacketID: f.PacketID}
		if f.PacketID != 0 {
			if cached, ok := socket.cachedResponse(key); ok {
				_ = socket.writeResponse(addr, query, cached)
				continue
			}
		}
		if f.Type == frameSYN && socket.accepting {
			socket.handleSYN(addr, query, f)
			continue
		}
		socket.mu.Lock()
		conn := socket.conns[f.SessionID]
		socket.mu.Unlock()
		if conn == nil {
			continue
		}
		conn.handleFrame(f)
		wait := time.Duration(0)
		if f.Type == framePOLL {
			wait = socket.config.PendingWait
		}
		response := conn.nextResponse(wait)
		if f.PacketID != 0 {
			socket.storeResponse(key, response)
		}
		_ = socket.writeResponse(addr, query, response)
	}
}

func (socket *packetSocket) handleSYN(peer *net.UDPAddr, query dnsQuery, f frame) {
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
			conn.closeWithError(errors.New("DNS accept queue full"))
			_ = socket.writeResponse(peer, query, frame{Type: frameRESET, SessionID: f.SessionID})
			return
		}
	}
	socket.mu.Unlock()
	response := frame{
		Type:      frameSYNACK,
		SessionID: f.SessionID,
		Ack:       conn.recvAck(),
	}
	if f.PacketID != 0 {
		socket.storeResponse(cacheKey{SessionID: f.SessionID, SenderID: f.SenderID, PacketID: f.PacketID}, response)
	}
	_ = socket.writeResponse(peer, query, response)
}

func (socket *packetSocket) writeResponse(peer *net.UDPAddr, query dnsQuery, f frame) error {
	f.SenderID = socket.endpointID
	if f.SessionID == 0 {
		return errors.New("DNS response frame missing session id")
	}
	payload, err := encodeFrame(f)
	if err != nil {
		return err
	}
	raw, err := buildResponse(query, payload, socket.config)
	if err != nil {
		return err
	}
	_, err = socket.pc.WriteToUDP(raw, peer)
	return err
}

func (socket *packetSocket) exchange(ctx context.Context, f frame) (frame, error) {
	if socket.resolver == nil {
		return frame{}, errors.New("DNS resolver is not configured")
	}
	socket.exchangeM.Lock()
	defer socket.exchangeM.Unlock()

	f.SenderID = socket.endpointID
	f.PacketID = socket.packetID.Add(1)
	payload, err := encodeFrame(f)
	if err != nil {
		return frame{}, err
	}
	query, id, err := buildQuery(socket.domain, payload, socket.config)
	if err != nil {
		return frame{}, err
	}
	deadline := time.Now().Add(socket.config.QueryTimeout)
	if ctx != nil {
		if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
			deadline = ctxDeadline
		}
	}
	if err := socket.pc.SetDeadline(deadline); err != nil {
		return frame{}, err
	}
	if _, err := socket.pc.WriteToUDP(query, socket.resolver); err != nil {
		return frame{}, err
	}

	buf := make([]byte, 64*1024)
	for {
		n, _, err := socket.pc.ReadFromUDP(buf)
		if err != nil {
			return frame{}, err
		}
		rawPayload, err := parseResponse(buf[:n], id)
		if err != nil {
			if strings.Contains(err.Error(), "id mismatch") {
				continue
			}
			return frame{}, err
		}
		resp, err := decodeFrame(rawPayload)
		if err != nil {
			return frame{}, err
		}
		if resp.SessionID != f.SessionID {
			continue
		}
		if resp.SenderID == socket.endpointID {
			continue
		}
		return resp, nil
	}
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

func udpAddrString(addr *net.UDPAddr) string {
	if addr == nil {
		return ""
	}
	return addr.String()
}

func timeoutContext(deadline time.Time, fallback time.Duration) (context.Context, context.CancelFunc) {
	if !deadline.IsZero() {
		return context.WithDeadline(context.Background(), deadline)
	}
	return context.WithTimeout(context.Background(), fallback)
}

type timeoutError string

func (err timeoutError) Error() string {
	return string(err)
}

func (err timeoutError) Timeout() bool {
	return true
}

func (err timeoutError) Temporary() bool {
	return true
}

func wrapDNSTimeout(err error, context string) error {
	if err == nil {
		return nil
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return timeoutError(context)
	}
	return err
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}
	return false
}

func configForDomain(domain string, config Config) (Config, error) {
	config = normalizeConfig(config)
	maxPayload, err := maxPayloadMTUForDomain(domain, config)
	if err != nil {
		return config, err
	}
	if config.PayloadMTU > maxPayload {
		config.PayloadMTU = maxPayload
	}
	if config.PayloadMTU <= 0 {
		return config, errors.New("DNS payload MTU is too small for selected domain and query length")
	}
	return config, nil
}

type cacheKey struct {
	SessionID uint64
	SenderID  uint64
	PacketID  uint64
}

func (socket *packetSocket) cachedResponse(key cacheKey) (frame, bool) {
	socket.cacheMu.Lock()
	defer socket.cacheMu.Unlock()
	if socket.cache == nil || key.PacketID == 0 {
		return frame{}, false
	}
	f, ok := socket.cache[key]
	if !ok {
		return frame{}, false
	}
	return cloneFrame(f), true
}

func (socket *packetSocket) storeResponse(key cacheKey, response frame) {
	if key.PacketID == 0 || socket.config.ResponseCache <= 0 {
		return
	}
	socket.cacheMu.Lock()
	defer socket.cacheMu.Unlock()
	if socket.cache == nil {
		socket.cache = make(map[cacheKey]frame)
	}
	if _, exists := socket.cache[key]; !exists {
		socket.cacheOrder = append(socket.cacheOrder, key)
	}
	socket.cache[key] = cloneFrame(response)
	for len(socket.cacheOrder) > socket.config.ResponseCache {
		oldest := socket.cacheOrder[0]
		copy(socket.cacheOrder, socket.cacheOrder[1:])
		socket.cacheOrder = socket.cacheOrder[:len(socket.cacheOrder)-1]
		delete(socket.cache, oldest)
	}
}

func cloneFrame(in frame) frame {
	out := in
	if in.Payload != nil {
		out.Payload = append([]byte(nil), in.Payload...)
	}
	return out
}
