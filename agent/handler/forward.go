package handler

import (
	"net"
	"time"

	"TengShe/agent/manager"
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
	"TengShe/share"
)

type Forward struct {
	Seq  uint64
	Addr string
}

func newForward(seq uint64, addr string) *Forward {
	forward := new(Forward)
	forward.Seq = seq
	forward.Addr = addr
	return forward
}

func (forward *Forward) start(mgr *manager.Manager) {
	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	finHeader := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.FORWARDFIN,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	finMess := &protocol.ForwardFin{
		Seq: forward.Seq,
	}

	defer func() {
		protocol.SendMessage(sMessage, finHeader, finMess, false)
	}()

	conn, err := net.DialTimeout("tcp", forward.Addr, 10*time.Second)
	if err != nil {
		return
	}
	share.ConfigureTCPConn(conn)

	task := &manager.ForwardTask{
		Mode: manager.F_CHECKFORWARD,
		Seq:  forward.Seq,
	}

	mgr.ForwardManager.TaskChan <- task
	result := <-mgr.ForwardManager.ResultChan
	if !result.OK {
		share.CloseQuietly(conn)
		return
	}

	task = &manager.ForwardTask{
		Mode: manager.F_GETDATACHAN,
		Seq:  forward.Seq,
	}
	mgr.ForwardManager.TaskChan <- task
	result = <-mgr.ForwardManager.ResultChan
	if !result.OK { // no need to close conn,cuz conn has been already recorded,so if FIN occur between F_UPDATEFORWARD and F_GETDATACHAN,closeTCP will help us to close the conn
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
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.FORWARDDATA,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
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
			Seq:     forward.Seq,
			DataLen: uint64(length),
			Data:    buffer[:length],
		}

		protocol.SendMessage(sMessage, dataHeader, forwardDataMess, false)
	}
}

func testForward(addr string) {
	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	header := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.FORWARDREADY,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	succMess := &protocol.ForwardReady{
		OK: 1,
	}

	failMess := &protocol.ForwardReady{
		OK: 0,
	}

	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		protocol.SendMessage(sMessage, header, failMess, false)
		return
	}

	share.CloseQuietly(conn)

	protocol.SendMessage(sMessage, header, succMess, false)
}

func DispatchForwardMess(mgr *manager.Manager) {
	for {
		message := <-mgr.ForwardManager.ForwardMessChan

		switch mess := message.(type) {
		case *protocol.ForwardTest:
			go testForward(mess.Addr)
		case *protocol.ForwardStart:
			task := &manager.ForwardTask{
				Mode: manager.F_NEWFORWARD,
				Seq:  mess.Seq,
			}
			mgr.ForwardManager.TaskChan <- task
			<-mgr.ForwardManager.ResultChan
			forward := newForward(mess.Seq, mess.Addr)
			go forward.start(mgr)
		case *protocol.ForwardData:
			mgrTask := &manager.ForwardTask{
				Mode: manager.F_GETDATACHAN,
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
