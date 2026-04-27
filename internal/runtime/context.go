package runtime

import (
	"net"
	"strings"
	"sync"

	"TengShe/global"
	"TengShe/protocol"
)

type Context struct {
	Component   *protocol.MessageComponent
	TLSEnable   bool
	Sender      *protocol.Sender
	downstreams map[string]*downstreamConn
	mu          sync.RWMutex
}

type downstreamConn struct {
	conn   net.Conn
	sender *protocol.Sender
}

var current *Context

func NewContext(conn net.Conn, secret, uuid string, tlsEnable bool) *Context {
	ctx := &Context{
		Component: &protocol.MessageComponent{
			Secret: secret,
			Conn:   conn,
			UUID:   uuid,
		},
		TLSEnable:   tlsEnable,
		downstreams: make(map[string]*downstreamConn),
	}
	if conn != nil {
		ctx.Sender = protocol.NewSender(conn)
	}
	return ctx
}

func (ctx *Context) ApplyGlobal() {
	if ctx == nil || ctx.Component == nil {
		return
	}
	current = ctx
	global.G_Component = ctx.Component
	global.G_TLSEnable = ctx.TLSEnable
}

func Current() *Context {
	if current != nil {
		return current
	}

	return &Context{
		Component:   global.G_Component,
		TLSEnable:   global.G_TLSEnable,
		downstreams: make(map[string]*downstreamConn),
	}
}

func Component() *protocol.MessageComponent {
	return Current().Component
}

func TLSEnabled() bool {
	return Current().TLSEnable
}

func SendControl(message protocol.Message, header *protocol.Header, mess interface{}, isPass bool) error {
	return send(protocol.ControlLane, message, header, mess, isPass)
}

func SendData(message protocol.Message, header *protocol.Header, mess interface{}, isPass bool) error {
	return send(protocol.DataLane, message, header, mess, isPass)
}

func SendAuto(message protocol.Message, header *protocol.Header, mess interface{}, isPass bool) error {
	return send(protocol.LaneForMessageType(header.MessageType), message, header, mess, isPass)
}

func NewDownstreamMessage(accepter string, route string) protocol.Message {
	component := Component()
	return protocol.NewDownMsg(DownstreamConn(accepter, route), component.Secret, component.UUID)
}

func RegisterDownstreamConn(uuid string, conn net.Conn) {
	Current().RegisterDownstreamConn(uuid, conn)
}

func UnregisterDownstreamConn(uuid string) {
	Current().UnregisterDownstreamConn(uuid)
}

func UnregisterDownstreamConnIfMatch(uuid string, conn net.Conn) bool {
	return Current().UnregisterDownstreamConnIfMatch(uuid, conn)
}

func DownstreamConn(accepter string, route string) net.Conn {
	return Current().DownstreamConn(accepter, route)
}

func UpdateConn(conn net.Conn) {
	Current().UpdateConn(conn)
}

func (ctx *Context) UpdateConn(conn net.Conn) {
	if ctx == nil || ctx.Component == nil {
		return
	}
	if ctx.Sender != nil {
		ctx.Sender.Close()
	}
	ctx.Component.Conn = conn
	if conn != nil {
		ctx.Sender = protocol.NewSender(conn)
	} else {
		ctx.Sender = nil
	}
	global.UpdateGComponent(conn)
}

func (ctx *Context) RegisterDownstreamConn(uuid string, conn net.Conn) {
	if ctx == nil || uuid == "" || conn == nil {
		return
	}
	ctx.ensureDownstreams()

	var old *downstreamConn
	ctx.mu.Lock()
	if current := ctx.downstreams[uuid]; current != nil && current.conn != conn {
		old = current
	}
	sender := protocol.SenderForConn(conn)
	if sender == nil {
		sender = protocol.NewSender(conn)
	}
	ctx.downstreams[uuid] = &downstreamConn{conn: conn, sender: sender}
	ctx.mu.Unlock()

	closeDownstream(old)
}

func (ctx *Context) UnregisterDownstreamConn(uuid string) {
	if ctx == nil || uuid == "" {
		return
	}

	ctx.mu.Lock()
	current := ctx.downstreams[uuid]
	delete(ctx.downstreams, uuid)
	ctx.mu.Unlock()

	closeDownstream(current)
}

func (ctx *Context) UnregisterDownstreamConnIfMatch(uuid string, conn net.Conn) bool {
	if ctx == nil || uuid == "" || conn == nil {
		return false
	}

	var current *downstreamConn
	ctx.mu.Lock()
	if existing := ctx.downstreams[uuid]; existing != nil && existing.conn == conn {
		current = existing
		delete(ctx.downstreams, uuid)
	}
	ctx.mu.Unlock()

	closeDownstream(current)
	return current != nil
}

func (ctx *Context) DownstreamConn(accepter string, route string) net.Conn {
	if ctx == nil || ctx.Component == nil {
		return nil
	}

	nextHop := nextHopForRoute(accepter, route)
	ctx.mu.RLock()
	if current := ctx.downstreams[nextHop]; current != nil {
		conn := current.conn
		ctx.mu.RUnlock()
		return conn
	}
	ctx.mu.RUnlock()

	return ctx.Component.Conn
}

func (ctx *Context) ensureDownstreams() {
	ctx.mu.Lock()
	if ctx.downstreams == nil {
		ctx.downstreams = make(map[string]*downstreamConn)
	}
	ctx.mu.Unlock()
}

func nextHopForRoute(accepter string, route string) string {
	if route == "" || route == protocol.TEMP_ROUTE {
		return accepter
	}
	nextHop, _, ok := strings.Cut(route, ":")
	if ok {
		return nextHop
	}
	return route
}

func closeDownstream(conn *downstreamConn) {
	if conn == nil {
		return
	}
	if conn.sender != nil {
		conn.sender.Close()
	}
	if conn.conn != nil {
		_ = conn.conn.Close()
	}
}

func send(lane protocol.Lane, message protocol.Message, header *protocol.Header, mess interface{}, isPass bool) error {
	ctx := Current()
	if ctx == nil || ctx.Sender == nil {
		protocol.SendMessage(message, header, mess, isPass)
		return nil
	}
	return ctx.Sender.SendMessage(lane, message, header, mess, isPass)
}
