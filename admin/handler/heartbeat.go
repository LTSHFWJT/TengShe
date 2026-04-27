package handler

import (
	"TengShe/admin/topology"
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
	"time"
)

func LetHeartbeat(topo *topology.Topology) {
	for {
		time.Sleep(time.Duration(10) * time.Second)

		routes := topo.QueryAllRouteInfo()
		for _, routeInfo := range routes {
			if routeInfo.Target == "" {
				continue
			}

			sMessage := tsruntime.NewDownstreamMessage(routeInfo.Target, routeInfo.Wire)

			header := &protocol.Header{
				Sender:      protocol.ADMIN_UUID,
				Accepter:    routeInfo.Target,
				MessageType: protocol.HEARTBEAT,
				RouteLen:    uint32(len([]byte(routeInfo.Wire))),
				Route:       routeInfo.Wire,
			}

			HBMess := &protocol.HeartbeatMsg{
				Ping: 1,
			}

			protocol.ConstructMessage(sMessage, header, HBMess, false)
			sMessage.SendMessage()
		}
	}
}
