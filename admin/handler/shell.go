package handler

import (
	"TengShe/admin/manager"
	"TengShe/admin/printer"
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
)

func LetShellStart(route string, uuid string) {
	sMessage := tsruntime.NewDownstreamMessage(uuid, route)

	header := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    uuid,
		MessageType: protocol.SHELLREQ,
		RouteLen:    uint32(len([]byte(route))),
		Route:       route,
	}

	shellReqMess := &protocol.ShellReq{
		Start: 1,
	}

	protocol.ConstructMessage(sMessage, header, shellReqMess, false)
	sMessage.SendMessage()
}

func DispatchShellMess(mgr *manager.Manager) {
	for {
		message := <-mgr.ShellManager.ShellMessChan

		switch mess := message.(type) {
		case *protocol.ShellRes:
			if mess.OK == 1 {
				mgr.ConsoleManager.OK <- true
			} else {
				mgr.ConsoleManager.OK <- false
			}
		case *protocol.ShellResult:
			printer.Print("%s", mess.Result)
		case *protocol.ShellExit:
			mgr.ConsoleManager.Exit <- true
		}
	}
}
