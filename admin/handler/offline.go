package handler

import (
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
)

func LetShutdown(route string, uuid string) {
	sMessage := tsruntime.NewDownstreamMessage(uuid, route)

	header := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    uuid,
		MessageType: protocol.SHUTDOWN,
		RouteLen:    uint32(len([]byte(route))),
		Route:       route,
	}

	shutdownMess := &protocol.Shutdown{
		OK: 1,
	}

	protocol.ConstructMessage(sMessage, header, shutdownMess, false)
	sMessage.SendMessage()
}
