package initial

import (
	"net"
	"os"

	"TengShe/admin/printer"
	"TengShe/admin/topology"
	"TengShe/protocol"
	"TengShe/share"
	"TengShe/utils"
)

type AdminConn struct {
	UUID  string
	Conn  net.Conn
	IDNum int
}

func dispatchUUID(conn net.Conn, secret string) string {
	var sMessage protocol.Message

	uuid := utils.GenerateUUID()
	uuidMess := &protocol.UUIDMess{
		UUIDLen: uint16(len(uuid)),
		UUID:    uuid,
	}

	header := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    protocol.TEMP_UUID,
		MessageType: protocol.UUID,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	sMessage = protocol.NewDownMsg(conn, secret, protocol.ADMIN_UUID)

	protocol.SendMessage(sMessage, header, uuidMess, false)

	return uuid
}

func NormalActive(userOptions *Options, topo *topology.Topology, proxy share.Proxy) *AdminConn {

	var sMessage, rMessage protocol.Message

	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len("Shhh...")),
		Greeting:    "Shhh...",
		UUIDLen:     uint16(len(protocol.ADMIN_UUID)),
		UUID:        protocol.ADMIN_UUID,
		IsAdmin:     1,
		IsReconnect: 0,
	}

	header := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    protocol.TEMP_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	for {
		var (
			conn net.Conn
			err  error
		)

		if proxy == nil {
			conn, err = net.Dial("tcp", userOptions.Connect)
		} else {
			conn, err = proxy.Dial()
		}

		if err != nil {
			printer.Fail("[*] Error occurred: %s", err.Error())
			os.Exit(0)
		}

		conn, tlsWrapped, err := share.PrepareActiveDownstreamConn(conn, userOptions.TlsEnable, userOptions.Domain)
		if tlsWrapped {
			userOptions.Secret = ""
		}
		if err != nil {
			printer.Fail("[*] Error occurred: %s", err.Error())
			share.CloseQuietly(conn)
			if share.IsConnPrepareStage(err, share.ConnPrepareStageTLS) {
				continue
			}
			os.Exit(0)
		}

		sMessage = protocol.NewDownMsg(conn, userOptions.Secret, protocol.ADMIN_UUID)

		protocol.SendMessage(sMessage, header, hiMess, false)

		rMessage = protocol.NewDownMsg(conn, userOptions.Secret, protocol.ADMIN_UUID)
		fHeader, fMessage, err := protocol.DestructMessage(rMessage)

		if err != nil {
			share.CloseQuietly(conn)
			printer.Fail("[*] Fail to connect node %s, Error: %s", conn.RemoteAddr().String(), err.Error())
			os.Exit(0)
		}

		if fHeader.MessageType == protocol.HI {
			mmess := fMessage.(*protocol.HIMess)
			if mmess.Greeting == "Keep silent" && mmess.IsAdmin == 0 {
				if mmess.IsReconnect == 0 {
					uuid := dispatchUUID(conn, userOptions.Secret)
					node := topology.NewNode(uuid, conn.RemoteAddr().String())
					_, idNum := topo.AddNode(node, protocol.TEMP_UUID, true)

					printer.Success("[*] Connect to node %s successfully! Node id is %d\r\n", conn.RemoteAddr().String(), idNum)
					return &AdminConn{UUID: uuid, Conn: conn, IDNum: idNum}
				} else {
					node := topology.NewNode(mmess.UUID, conn.RemoteAddr().String())
					_, idNum := topo.AddNode(node, protocol.TEMP_UUID, true)

					printer.Success("[*] Connect to node %s successfully! Node id is %d\r\n", conn.RemoteAddr().String(), idNum)
					return &AdminConn{UUID: mmess.UUID, Conn: conn, IDNum: idNum}
				}
			}
		}

		share.CloseQuietly(conn)
		printer.Fail("[*] Target node looks invalid!\n")
	}
}

func NormalPassive(userOptions *Options, topo *topology.Topology, accepted chan<- *AdminConn) *AdminConn {
	listenAddr, _, err := utils.CheckIPPort(userOptions.Listen)
	if err != nil {
		printer.Fail("[*] Error occurred: %s", err.Error())
		os.Exit(0)
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		printer.Fail("[*] Error occurred: %s", err.Error())
		os.Exit(0)
	}

	first := acceptPassiveConn(listener, userOptions, topo)
	go acceptPassiveLoop(listener, userOptions, topo, accepted)
	return first
}

func acceptPassiveLoop(listener net.Listener, userOptions *Options, topo *topology.Topology, accepted chan<- *AdminConn) {
	for {
		conn := acceptPassiveConn(listener, userOptions, topo)
		if accepted != nil {
			accepted <- conn
		}
	}
}

func acceptPassiveConn(listener net.Listener, userOptions *Options, topo *topology.Topology) *AdminConn {
	var sMessage, rMessage protocol.Message

	// just say hi!
	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len("Keep silent")),
		Greeting:    "Keep silent",
		UUIDLen:     uint16(len(protocol.ADMIN_UUID)),
		UUID:        protocol.ADMIN_UUID,
		IsAdmin:     1,
		IsReconnect: 0,
	}

	header := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    protocol.TEMP_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			printer.Fail("[*] Error occurred: %s\r\n", err.Error())
			continue
		}

		conn, tlsWrapped, err := share.PreparePassiveDownstreamConn(conn, userOptions.TlsEnable)
		if tlsWrapped {
			userOptions.Secret = ""
		}
		if err != nil {
			printer.Fail("[*] Error occurred: %s\r\n", err.Error())
			share.CloseQuietly(conn)
			continue
		}

		rMessage = protocol.NewDownMsg(conn, userOptions.Secret, protocol.ADMIN_UUID)
		fHeader, fMessage, err := protocol.DestructMessage(rMessage)

		if err != nil {
			printer.Fail("[*] Fail to set connection from %s, Error: %s\r\n", conn.RemoteAddr().String(), err.Error())
			share.CloseQuietly(conn)
			continue
		}

		if fHeader.MessageType == protocol.HI {
			mmess := fMessage.(*protocol.HIMess)
			if mmess.Greeting == "Shhh..." && mmess.IsAdmin == 0 {
				sMessage = protocol.NewDownMsg(conn, userOptions.Secret, protocol.ADMIN_UUID)
				protocol.SendMessage(sMessage, header, hiMess, false)

				if mmess.IsReconnect == 0 {
					uuid := dispatchUUID(conn, userOptions.Secret)
					node := topology.NewNode(uuid, conn.RemoteAddr().String())
					_, idNum := topo.AddNode(node, protocol.TEMP_UUID, true)

					printer.Success("[*] Connection from node %s is set up successfully! Node id is %d\r\n", conn.RemoteAddr().String(), idNum)
					return &AdminConn{UUID: uuid, Conn: conn, IDNum: idNum}
				} else {
					node := topology.NewNode(mmess.UUID, conn.RemoteAddr().String())
					_, idNum := topo.AddNode(node, protocol.TEMP_UUID, true)

					printer.Success("[*] Connection from node %s is set up successfully! Node id is %d\r\n", conn.RemoteAddr().String(), idNum)
					return &AdminConn{UUID: mmess.UUID, Conn: conn, IDNum: idNum}
				}
			}
		}

		share.CloseQuietly(conn)
		printer.Fail("[*] Incoming connection looks invalid.")
	}
}
