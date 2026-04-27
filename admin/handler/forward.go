package handler

import (
	"fmt"
	"net"

	"TengShe/admin/manager"
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
	"TengShe/share"
)

type Forward struct {
	Addr string
	Port string
}

func NewForward(port, addr string) *Forward {
	forward := new(Forward)
	forward.Port = port
	forward.Addr = addr
	return forward
}

func (forward *Forward) LetForward(mgr *manager.Manager, route string, uuid string) error {
	listenAddr := fmt.Sprintf("0.0.0.0:%s", forward.Port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}

	sMessage := tsruntime.NewDownstreamMessage(uuid, route)

	header := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    uuid,
		MessageType: protocol.FORWARDTEST,
		RouteLen:    uint32(len([]byte(route))),
		Route:       route,
	}

	testMess := &protocol.ForwardTest{
		AddrLen: uint16(len([]byte(forward.Addr))),
		Addr:    forward.Addr,
	}

	protocol.SendMessage(sMessage, header, testMess, false)

	if ready := <-mgr.ForwardManager.ForwardReady; !ready {
		share.CloseQuietly(listener)
		err := fmt.Errorf("fail to forward port %s to remote addr %s,remote addr is not responding", forward.Port, forward.Addr)
		return err
	}

	mgrTask := &manager.ForwardTask{
		Mode:       manager.F_NEWFORWARD,
		UUID:       uuid,
		Listener:   listener,
		Port:       forward.Port,
		RemoteAddr: forward.Addr,
	}

	mgr.ForwardManager.TaskChan <- mgrTask
	<-mgr.ForwardManager.ResultChan

	go forward.handleForwardListener(mgr, listener, route, uuid)

	return nil
}

func (forward *Forward) handleForwardListener(mgr *manager.Manager, listener net.Listener, route string, uuid string) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			share.CloseQuietly(listener) // todo:map没有释放
			return
		}
		share.ConfigureTCPConn(conn)

		mgrTask := &manager.ForwardTask{
			Mode: manager.F_GETNEWSEQ,
			UUID: uuid,
			Port: forward.Port,
		}
		mgr.ForwardManager.TaskChan <- mgrTask
		result := <-mgr.ForwardManager.ResultChan
		seq := result.ForwardSeq

		mgrTask = &manager.ForwardTask{
			Mode: manager.F_ADDCONN,
			UUID: uuid,
			Seq:  seq,
			Port: forward.Port,
		}
		mgr.ForwardManager.TaskChan <- mgrTask
		result = <-mgr.ForwardManager.ResultChan
		if !result.OK {
			share.CloseQuietly(conn)
			return
		}

		go forward.handleForward(mgr, conn, route, uuid, seq)
	}
}

func (forward *Forward) handleForward(mgr *manager.Manager, conn net.Conn, route string, uuid string, seq uint64) {
	sMessage := tsruntime.NewDownstreamMessage(uuid, route)
	// tell agent to start
	startHeader := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    uuid,
		MessageType: protocol.FORWARDSTART,
		RouteLen:    uint32(len([]byte(route))),
		Route:       route,
	}

	startMess := &protocol.ForwardStart{
		Seq:     seq,
		AddrLen: uint16(len([]byte(forward.Addr))),
		Addr:    forward.Addr,
	}

	// Fetch the session data channel before notifying the agent so early return
	// data has a registered destination.
	mgrTask := &manager.ForwardTask{
		Mode: manager.F_GETDATACHAN,
		UUID: uuid,
		Seq:  seq,
		Port: forward.Port,
	}
	mgr.ForwardManager.TaskChan <- mgrTask
	result := <-mgr.ForwardManager.ResultChan

	protocol.SendMessage(sMessage, startHeader, startMess, false)

	defer func() {
		finHeader := &protocol.Header{
			Sender:      protocol.ADMIN_UUID,
			Accepter:    uuid,
			MessageType: protocol.FORWARDFIN,
			RouteLen:    uint32(len([]byte(route))),
			Route:       route,
		}

		finMess := &protocol.ForwardFin{
			Seq: seq,
		}

		protocol.SendMessage(sMessage, finHeader, finMess, false)
	}()

	if !result.OK {
		return
	}

	dataChan := result.DataChan

	go func() {
		for {
			if data, ok := <-dataChan; ok {
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

	dataHeader := &protocol.Header{
		Sender:      protocol.ADMIN_UUID,
		Accepter:    uuid,
		MessageType: protocol.FORWARDDATA,
		RouteLen:    uint32(len([]byte(route))),
		Route:       route,
	}

	buffer := share.AcquireTransferBuffer()
	defer share.ReleaseTransferBuffer(buffer)

	for {
		length, err := conn.Read(buffer)
		if err != nil {
			share.CloseQuietly(conn)
			return
		}

		forwardDataMess := &protocol.ForwardData{
			Seq:     seq,
			DataLen: uint64(length),
			Data:    buffer[:length],
		}

		protocol.SendMessage(sMessage, dataHeader, forwardDataMess, false)
	}
}

func GetForwardInfo(mgr *manager.Manager, uuid string) (int, bool) {
	mgrTask := &manager.ForwardTask{
		Mode: manager.F_GETFORWARDINFO,
		UUID: uuid,
	}
	mgr.ForwardManager.TaskChan <- mgrTask
	result := <-mgr.ForwardManager.ResultChan

	if result.OK {
		fmt.Print("\r\n[0] All")
		for _, info := range result.ForwardInfo {
			fmt.Printf(
				"\r\n[%d] Listening Addr: %s , Remote Addr: %s , Active Connections: %d",
				info.Seq,
				info.Laddr,
				info.Raddr,
				info.ActiveNum,
			)
		}
	}

	return len(result.ForwardInfo) - 1, result.OK
}

func StopForward(mgr *manager.Manager, uuid string, choice int) {
	if choice == 0 {
		mgrTask := &manager.ForwardTask{
			Mode: manager.F_CLOSESINGLEALL,
			UUID: uuid,
		}
		mgr.ForwardManager.TaskChan <- mgrTask
		<-mgr.ForwardManager.ResultChan
	} else {
		mgrTask := &manager.ForwardTask{
			Mode:        manager.F_CLOSESINGLE,
			UUID:        uuid,
			CloseTarget: choice,
		}
		mgr.ForwardManager.TaskChan <- mgrTask
		<-mgr.ForwardManager.ResultChan
	}
}

func DispatchForwardMess(mgr *manager.Manager) {
	for {
		message := <-mgr.ForwardManager.ForwardMessChan

		switch mess := message.(type) {
		case *protocol.ForwardReady:
			if mess.OK == 1 {
				mgr.ForwardManager.ForwardReady <- true
			} else {
				mgr.ForwardManager.ForwardReady <- false
			}
		case *protocol.ForwardData:
			mgrTask := &manager.ForwardTask{
				Mode: manager.F_GETDATACHAN_WITHOUTUUID,
				Seq:  mess.Seq,
			}
			mgr.ForwardManager.TaskChan <- mgrTask
			result := <-mgr.ForwardManager.ResultChan
			if result.OK {
				if !share.EnqueueBytes(result.DataChan, mess.Data, share.DefaultDataEnqueueTimeout) {
					mgr.ForwardManager.TaskChan <- &manager.ForwardTask{
						Mode: manager.F_CLOSETCP,
						Seq:  mess.Seq,
					}
				}
			}
		case *protocol.ForwardFin:
			mgrTask := &manager.ForwardTask{
				Mode: manager.F_CLOSETCP,
				Seq:  mess.Seq,
			}
			mgr.ForwardManager.TaskChan <- mgrTask
		}
	}
}
