package handler

import (
	"context"
	"crypto/tls"
	"errors"
	"net"

	"TengShe/agent/manager"
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
	"TengShe/share"
	"TengShe/share/transport"
	"TengShe/share/transport/stream"
)

type Connect struct {
	Protocol string
	Addr     string
}

func newConnect(protocolName string, addr string) *Connect {
	connect := new(Connect)
	connect.Protocol = protocolName
	connect.Addr = addr
	return connect
}

func (connect *Connect) start(mgr *manager.Manager) {
	var sUMessage, sLMessage, rMessage protocol.Message

	sUMessage = protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	hiHeader := &protocol.Header{
		Sender:      protocol.ADMIN_UUID, // fake admin
		Accepter:    protocol.TEMP_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	// fake admin
	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len("Shhh...")),
		Greeting:    "Shhh...",
		UUIDLen:     uint16(len(protocol.ADMIN_UUID)),
		UUID:        protocol.ADMIN_UUID,
		IsAdmin:     1,
		IsReconnect: 0,
	}

	doneHeader := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.CONNECTDONE,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	doneSuccMess := &protocol.ConnectDone{
		OK: 1,
	}

	doneFailMess := &protocol.ConnectDone{
		OK: 0,
	}

	var (
		conn net.Conn
		err  error
	)

	defer func() {
		if err != nil {
			protocol.ConstructMessage(sUMessage, doneHeader, doneFailMess, false)
			sUMessage.SendMessage()
		}
	}()

	transportSpec, err := stream.Get(connect.Protocol)
	if err != nil {
		return
	}

	conn, err = transportSpec.Dial(context.Background(), connect.Addr, "")

	if err != nil {
		return
	}

	if tsruntime.TLSEnabled() && transportSpec.SupportsTLS() {
		var tlsConfig *tls.Config
		// Set domain as null since we are in the intranet
		tlsConfig, err = transport.NewClientTLSConfig("")
		if err != nil {
			share.CloseQuietly(conn)
			return
		}
		conn = transport.WrapTLSClientConn(conn, tlsConfig)
	}
	// There's no need for the "domain" parameter between intranet nodes
	param := new(protocol.NegParam)
	param.Conn = conn
	proto := protocol.NewDownProto(param)
	proto.CNegotiate()

	if err = share.ActivePreAuth(conn); err != nil {
		return
	}

	sLMessage = protocol.NewDownMsg(conn, tsruntime.Component().Secret, protocol.ADMIN_UUID)

	protocol.ConstructMessage(sLMessage, hiHeader, hiMess, false)
	sLMessage.SendMessage()

	rMessage = protocol.NewDownMsg(conn, tsruntime.Component().Secret, protocol.ADMIN_UUID)
	fHeader, fMessage, err := protocol.DestructMessage(rMessage)

	if err != nil {
		share.CloseQuietly(conn)
		return
	}

	var childUUID string

	if fHeader.MessageType == protocol.HI {
		mmess := fMessage.(*protocol.HIMess)
		if mmess.Greeting == "Keep silent" && mmess.IsAdmin == 0 {
			if mmess.IsReconnect == 0 {
				childIP := conn.RemoteAddr().String()

				cUUIDReqHeader := &protocol.Header{
					Sender:      tsruntime.Component().UUID,
					Accepter:    protocol.ADMIN_UUID,
					MessageType: protocol.CHILDUUIDREQ,
					RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
					Route:       protocol.TEMP_ROUTE,
				}

				cUUIDMess := &protocol.ChildUUIDReq{
					ParentUUIDLen: uint16(len(tsruntime.Component().UUID)),
					ParentUUID:    tsruntime.Component().UUID,
					IPLen:         uint16(len(childIP)),
					IP:            childIP,
				}

				protocol.ConstructMessage(sUMessage, cUUIDReqHeader, cUUIDMess, false)
				sUMessage.SendMessage()

				childUUID = <-mgr.ListenManager.ChildUUIDChan

				uuidHeader := &protocol.Header{
					Sender:      protocol.ADMIN_UUID, // Fake admin LOL
					Accepter:    protocol.TEMP_UUID,
					MessageType: protocol.UUID,
					RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
					Route:       protocol.TEMP_ROUTE,
				}

				uuidMess := &protocol.UUIDMess{
					UUIDLen: uint16(len(childUUID)),
					UUID:    childUUID,
				}

				protocol.ConstructMessage(sLMessage, uuidHeader, uuidMess, false)
				sLMessage.SendMessage()
			} else {
				reheader := &protocol.Header{
					Sender:      tsruntime.Component().UUID,
					Accepter:    protocol.ADMIN_UUID,
					MessageType: protocol.NODEREONLINE,
					RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
					Route:       protocol.TEMP_ROUTE,
				}

				reMess := &protocol.NodeReonline{
					ParentUUIDLen: uint16(len(tsruntime.Component().UUID)),
					ParentUUID:    tsruntime.Component().UUID,
					UUIDLen:       uint16(len(mmess.UUID)),
					UUID:          mmess.UUID,
					IPLen:         uint16(len(conn.RemoteAddr().String())),
					IP:            conn.RemoteAddr().String(),
				}

				protocol.ConstructMessage(sUMessage, reheader, reMess, false)
				sUMessage.SendMessage()

				childUUID = mmess.UUID
			}

			childrenTask := &manager.ChildrenTask{
				Mode: manager.C_NEWCHILD,
				UUID: childUUID,
				Conn: conn,
			}
			mgr.ChildrenManager.TaskChan <- childrenTask
			<-mgr.ChildrenManager.ResultChan

			mgr.ChildrenManager.ChildComeChan <- &manager.ChildInfo{UUID: childUUID, Conn: conn}

			protocol.ConstructMessage(sUMessage, doneHeader, doneSuccMess, false)
			sUMessage.SendMessage()

			return
		}
	}

	share.CloseQuietly(conn)
	err = errors.New("node looks invalid")
}

func DispatchConnectMess(mgr *manager.Manager) {
	for {
		message := <-mgr.ConnectManager.ConnectMessChan

		switch mess := message.(type) {
		case *protocol.ConnectStart:
			protocolName, err := stream.NormalizeProtocol(mess.Protocol)
			if err != nil {
				protocolName = stream.ProtocolTCP
			}
			connect := newConnect(protocolName, mess.Addr)
			go connect.start(mgr)
		}
	}
}
