package handler

import (
	"TengShe/admin/manager"
	"TengShe/admin/printer"
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
	"TengShe/share/transport/stream"
)

func LetConnect(mgr *manager.Manager, route, uuid, addr, protocolName string) error {
	protocolName, err := stream.NormalizeProtocol(protocolName)
	if err != nil {
		return err
	}
	normalAddr, err := stream.NormalizeDialAddress(protocolName, addr)
	if err != nil {
		return err
	}

	sMessage := tsruntime.NewDownstreamMessage(uuid, route)

	header := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    uuid,
		MessageType: protocol.CONNECTSTART,
		RouteLen:    uint32(len([]byte(route))),
		Route:       route,
	}

	connMess := &protocol.ConnectStart{
		ProtocolLen: uint16(len(protocolName)),
		Protocol:    protocolName,
		AddrLen:     uint16(len([]byte(normalAddr))),
		Addr:        normalAddr,
	}

	protocol.ConstructMessage(sMessage, header, connMess, false)
	sMessage.SendMessage()

	if ok := <-mgr.ConnectManager.ConnectReady; !ok {
		printer.Fail("\r\n[*] Cannot connect to node %s via %s", addr, protocolName)
	}

	return nil
}

func DispatchConnectMess(mgr *manager.Manager) {
	for {
		message := <-mgr.ConnectManager.ConnectMessChan

		switch mess := message.(type) {
		case *protocol.ConnectDone:
			if mess.OK == 1 {
				mgr.ConnectManager.ConnectReady <- true
			} else {
				mgr.ConnectManager.ConnectReady <- false
			}
		}
	}
}
