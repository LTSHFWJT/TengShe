package handler

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"TengShe/admin/manager"
	"TengShe/admin/topology"
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
	"TengShe/share"
)

type Socks struct {
	Username string
	Password string
	Addr     string
	Port     string
}

const socksDataEnqueueTimeout = 30 * time.Second

func NewSocks(param string) *Socks {
	socks := new(Socks)

	slice := strings.SplitN(param, ":", 2)

	if len(slice) < 2 {
		socks.Addr = "0.0.0.0"
		socks.Port = param
	} else {
		socks.Addr = slice[0]
		socks.Port = slice[1]
	}

	return socks
}

func (socks *Socks) LetSocks(mgr *manager.Manager, route string, uuid string) error {
	var addr string

	addr = fmt.Sprintf("%s:%s", socks.Addr, socks.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	// register a new SOCKS service
	mgrTask := &manager.SocksTask{
		Mode:             manager.S_NEWSOCKS,
		UUID:             uuid,
		SocksAddr:        socks.Addr,
		SocksPort:        socks.Port,
		SocksUsername:    socks.Username,
		SocksPassword:    socks.Password,
		SocksTCPListener: listener,
	}

	mgr.SocksManager.TaskChan <- mgrTask
	result := <-mgr.SocksManager.ResultChan // wait for "add" done
	if !result.OK {                         // node and socks service must be one-to-one
		err := errors.New("SOCKS is already running on the current node. Use 'stopsocks' to stop the existing one.")
		share.CloseQuietly(listener)
		return err
	}

	sMessage := tsruntime.NewDownstreamMessage(uuid, route)

	header := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    uuid,
		MessageType: protocol.SOCKSSTART,
		RouteLen:    uint32(len([]byte(route))),
		Route:       route,
	}

	socksStartMess := &protocol.SocksStart{
		UsernameLen: uint64(len([]byte(socks.Username))),
		Username:    socks.Username,
		PasswordLen: uint64(len([]byte(socks.Password))),
		Password:    socks.Password,
	}

	protocol.SendMessage(sMessage, header, socksStartMess, false)

	if ready := <-mgr.SocksManager.SocksReady; !ready {
		err := errors.New("failed to start socks. If you just stopped the socks service, please wait for cleanup to finish.")
		StopSocks(mgr, uuid)
		return err
	}

	go socks.handleSocksListener(mgr, listener, route, uuid)

	return nil
}

func (socks *Socks) handleSocksListener(mgr *manager.Manager, listener net.Listener, route string, uuid string) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			share.CloseQuietly(listener)
			return
		}
		share.ConfigureTCPConn(conn)

		// ask new seq num
		mgrTask := &manager.SocksTask{
			Mode: manager.S_GETNEWSEQ,
			UUID: uuid,
		}
		mgr.SocksManager.TaskChan <- mgrTask
		result := <-mgr.SocksManager.ResultChan
		seq := result.SocksSeq

		// save the socket
		mgrTask = &manager.SocksTask{
			Mode:           manager.S_ADDTCPSOCKET,
			UUID:           uuid,
			Seq:            seq,
			SocksTCPSocket: conn,
		}
		mgr.SocksManager.TaskChan <- mgrTask
		result = <-mgr.SocksManager.ResultChan
		if !result.OK {
			share.CloseQuietly(conn)
			return
		}

		// handle it!
		go socks.handleSocks(mgr, conn, route, uuid, seq)
	}
}

func (socks *Socks) handleSocks(mgr *manager.Manager, conn net.Conn, route string, uuid string, seq uint64) {
	sMessage := tsruntime.NewDownstreamMessage(uuid, route)

	header := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    uuid,
		MessageType: protocol.SOCKSTCPDATA,
		RouteLen:    uint32(len([]byte(route))),
		Route:       route,
	}

	mgrTask := &manager.SocksTask{
		Mode: manager.S_GETTCPDATACHAN,
		UUID: uuid,
		Seq:  seq,
	}
	mgr.SocksManager.TaskChan <- mgrTask
	result := <-mgr.SocksManager.ResultChan
	if !result.OK {
		return
	}

	tcpDataChan := result.TCPDataChan

	// handle data from dispatcher
	go func() {
		for {
			if data, ok := <-tcpDataChan; ok {
				if err := share.WriteFull(conn, data); err != nil {
					share.CloseQuietly(conn)
					return
				}
			} else {
				share.CloseQuietly(conn)
				return
			}
		}
	}()

	var sendSth bool

	// SendTCPFin after browser close the conn
	defer func() {
		// check if "sendSth" is true
		// if true, then tell agent that the conn is closed
		// but keep "handle received data" working to achieve socksdata from agent that still on the way
		// if false, don't tell agent and do cleanup alone
		if !sendSth {
			// call HandleTCPFin by myself
			mgrTask := &manager.SocksTask{
				Mode: manager.S_CLOSETCP,
				Seq:  seq,
			}
			mgr.SocksManager.TaskChan <- mgrTask
			return
		}

		finHeader := &protocol.Header{
			Sender:      protocol.ADMIN_UUID,
			Accepter:    uuid,
			MessageType: protocol.SOCKSTCPFIN,
			RouteLen:    uint32(len([]byte(route))),
			Route:       route,
		}
		finMess := &protocol.SocksTCPFin{
			Seq: seq,
		}

		protocol.SendMessage(sMessage, finHeader, finMess, false)
	}()

	// handling data that comes from browser
	buffer := share.AcquireTransferBuffer()
	defer share.ReleaseTransferBuffer(buffer)

	// try to receive first packet
	// avoid browser to close the conn but sends nothing
	length, err := conn.Read(buffer)
	if err != nil {
		share.CloseQuietly(conn) // close conn immediately
		return
	}

	socksDataMess := &protocol.SocksTCPData{
		Seq:     seq,
		DataLen: uint64(length),
		Data:    buffer[:length],
	}

	protocol.SendMessage(sMessage, header, socksDataMess, false)

	// browser sends sth, so handling conn normally and setting sendSth->true
	for {
		length, err := conn.Read(buffer)
		if err != nil {
			sendSth = true
			share.CloseQuietly(conn) // close conn immediately,in case of sth is sent after TCPFin
			return
		}

		socksDataMess := &protocol.SocksTCPData{
			Seq:     seq,
			DataLen: uint64(length),
			Data:    buffer[:length],
		}

		protocol.SendMessage(sMessage, header, socksDataMess, false)
	}
}

func startUDPAss(mgr *manager.Manager, topo *topology.Topology, seq uint64) {
	var (
		err             error
		udpListenerAddr *net.UDPAddr
		udpListener     *net.UDPConn
	)

	mgrTask := &manager.SocksTask{
		Mode: manager.S_GETUDPSTARTINFO,
		Seq:  seq,
	}
	mgr.SocksManager.TaskChan <- mgrTask
	socksResult := <-mgr.SocksManager.ResultChan
	uuid := socksResult.UUID

	route := topo.QueryRoute(uuid)

	header := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    uuid,
		MessageType: protocol.UDPASSRES,
		RouteLen:    uint32(len([]byte(route))),
		Route:       route,
	}

	failMess := &protocol.UDPAssRes{
		Seq: seq,
		OK:  0,
	}

	sMessage := tsruntime.NewDownstreamMessage(uuid, route)

	defer func() {
		if err != nil {
			protocol.SendMessage(sMessage, header, failMess, false)
		}
	}()

	if socksResult.OK {
		udpListenerAddr, err = net.ResolveUDPAddr("udp", socksResult.TCPAddr+":0")
		if err != nil {
			return
		}

		udpListener, err = net.ListenUDP("udp", udpListenerAddr)
		if err != nil {
			return
		}

		mgrTask = &manager.SocksTask{
			Mode:             manager.S_UPDATEUDP,
			Seq:              seq,
			UUID:             uuid,
			SocksUDPListener: udpListener,
		}
		mgr.SocksManager.TaskChan <- mgrTask
		socksResult = <-mgr.SocksManager.ResultChan
		if !socksResult.OK {
			err = errors.New("TCP conn seems disconnected")
			return
		}

		go handleUDPAss(mgr, udpListener, route, uuid, seq)

		succMess := &protocol.UDPAssRes{
			Seq:     seq,
			OK:      1,
			AddrLen: uint16(len(udpListener.LocalAddr().String())),
			Addr:    udpListener.LocalAddr().String(),
		}

		protocol.SendMessage(sMessage, header, succMess, false)
	} else {
		err = errors.New("TCP conn seems disconnected")
		return
	}
}

func handleUDPAss(mgr *manager.Manager, listener *net.UDPConn, route string, uuid string, seq uint64) {
	sMessage := tsruntime.NewDownstreamMessage(uuid, route)

	dataHeader := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    uuid,
		MessageType: protocol.SOCKSUDPDATA,
		RouteLen:    uint32(len([]byte(route))),
		Route:       route,
	}

	mgrTask := &manager.SocksTask{
		Mode: manager.S_GETUDPDATACHAN,
		UUID: uuid,
		Seq:  seq,
	}
	mgr.SocksManager.TaskChan <- mgrTask
	result := <-mgr.SocksManager.ResultChan

	if !result.OK {
		return
	}

	udpDataChan := result.UDPDataChan

	buffer := share.AcquireTransferBuffer()
	defer share.ReleaseTransferBuffer(buffer)

	var alreadyGetAddr bool
	for {
		length, addr, err := listener.ReadFromUDP(buffer)
		if !alreadyGetAddr {
			go func() {
				for {
					if data, ok := <-udpDataChan; ok {
						if _, err := listener.WriteToUDP(data, addr); err != nil {
							share.CloseQuietly(listener)
							return
						}
					} else {
						share.CloseQuietly(listener)
						return
					}
				}
			}()
			alreadyGetAddr = true
		}

		if err != nil {
			share.CloseQuietly(listener)
			return
		}

		udpDataMess := &protocol.SocksUDPData{
			Seq:     seq,
			DataLen: uint64(length),
			Data:    buffer[:length],
		}

		protocol.SendMessage(sMessage, dataHeader, udpDataMess, false)
	}
}

func GetSocksInfo(mgr *manager.Manager, uuid string) bool {
	mgrTask := &manager.SocksTask{
		Mode: manager.S_GETSOCKSINFO,
		UUID: uuid,
	}
	mgr.SocksManager.TaskChan <- mgrTask
	result := <-mgr.SocksManager.ResultChan

	if result.OK {
		fmt.Printf(
			"\r\nSocks Info ---> ListenAddr: %s:%s    Username: %s   Password: %s",
			result.SocksInfo.Addr,
			result.SocksInfo.Port,
			result.SocksInfo.Username,
			result.SocksInfo.Password,
		)
	}

	return result.OK
}

func StopSocks(mgr *manager.Manager, uuid string) {
	mgrTask := &manager.SocksTask{
		Mode: manager.S_CLOSESOCKS,
		UUID: uuid,
	}
	mgr.SocksManager.TaskChan <- mgrTask
	<-mgr.SocksManager.ResultChan
}

func DispathSocksMess(mgr *manager.Manager, topo *topology.Topology) {
	for {
		message := <-mgr.SocksManager.SocksMessChan

		switch mess := message.(type) {
		case *protocol.SocksReady:
			if mess.OK == 1 {
				mgr.SocksManager.SocksReady <- true
			} else {
				mgr.SocksManager.SocksReady <- false
			}
		case *protocol.SocksTCPData:
			mgrTask := &manager.SocksTask{
				Mode: manager.S_GETTCPDATACHAN_WITHOUTUUID,
				Seq:  mess.Seq,
			}
			mgr.SocksManager.TaskChan <- mgrTask
			result := <-mgr.SocksManager.ResultChan
			if result.OK {
				if !enqueueSocksData(result.TCPDataChan, mess.Data) {
					mgr.SocksManager.TaskChan <- &manager.SocksTask{
						Mode: manager.S_CLOSETCP,
						Seq:  mess.Seq,
					}
				}
			}
		case *protocol.SocksTCPFin:
			mgrTask := &manager.SocksTask{
				Mode: manager.S_CLOSETCP,
				Seq:  mess.Seq,
			}
			mgr.SocksManager.TaskChan <- mgrTask
		case *protocol.UDPAssStart:
			go startUDPAss(mgr, topo, mess.Seq)
		case *protocol.SocksUDPData:
			mgrTask := &manager.SocksTask{
				Mode: manager.S_GETUDPDATACHAN_WITHOUTUUID,
				Seq:  mess.Seq,
			}
			mgr.SocksManager.TaskChan <- mgrTask
			result := <-mgr.SocksManager.ResultChan
			if result.OK {
				if !enqueueSocksData(result.UDPDataChan, mess.Data) {
					mgr.SocksManager.TaskChan <- &manager.SocksTask{
						Mode: manager.S_CLOSETCP,
						Seq:  mess.Seq,
					}
				}
			}
		}
	}
}

func enqueueSocksData(ch chan []byte, data []byte) (ok bool) {
	if ch == nil {
		return false
	}

	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	select {
	case ch <- data:
		return true
	default:
	}

	timer := time.NewTimer(socksDataEnqueueTimeout)
	defer timer.Stop()

	select {
	case ch <- data:
		return true
	case <-timer.C:
		return false
	}
}
