package handler

import (
	"fmt"
	"net"

	"TengShe/agent/manager"
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
	"TengShe/share"
)

type Backward struct {
	Lport    string
	Rport    string
	Listener net.Listener
}

func newBackward(listener net.Listener, lPort, rPort string) *Backward {
	backward := new(Backward)
	backward.Listener = listener
	backward.Lport = lPort
	backward.Rport = rPort
	return backward
}

func (backward *Backward) start(mgr *manager.Manager) {
	mgrTask := &manager.BackwardTask{
		Mode:     manager.B_NEWBACKWARD,
		Listener: backward.Listener,
		RPort:    backward.Rport,
	}

	mgr.BackwardManager.TaskChan <- mgrTask
	<-mgr.BackwardManager.ResultChan

	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	for {
		conn, err := backward.Listener.Accept()
		if err != nil {
			share.CloseQuietly(backward.Listener) // todo:closebackward消息处理
			return
		}
		share.ConfigureTCPConn(conn)

		seqHeader := &protocol.Header{
			Sender:      tsruntime.Component().UUID,
			Accepter:    protocol.ADMIN_UUID,
			MessageType: protocol.BACKWARDSTART,
			RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
			Route:       protocol.TEMP_ROUTE,
		}

		seqMess := &protocol.BackwardStart{
			UUIDLen:  uint16(len(tsruntime.Component().UUID)),
			UUID:     tsruntime.Component().UUID,
			LPortLen: uint16(len(backward.Lport)),
			LPort:    backward.Lport,
			RPortLen: uint16(len(backward.Rport)),
			RPort:    backward.Rport,
		}

		protocol.SendMessage(sMessage, seqHeader, seqMess, false)

		mgrTask = &manager.BackwardTask{
			Mode:  manager.B_GETSEQCHAN,
			RPort: backward.Rport,
		}
		mgr.BackwardManager.TaskChan <- mgrTask
		result := <-mgr.BackwardManager.ResultChan
		if !result.OK {
			share.CloseQuietly(conn)
			return
		}

		seq, ok := <-result.SeqChan
		if !ok {
			share.CloseQuietly(conn)
			return
		}

		go backward.handleBackward(mgr, conn, seq)
	}
}

func (backward *Backward) handleBackward(mgr *manager.Manager, conn net.Conn, seq uint64) {
	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	defer func() {
		finHeader := &protocol.Header{
			Sender:      tsruntime.Component().UUID,
			Accepter:    protocol.ADMIN_UUID,
			MessageType: protocol.BACKWARDFIN,
			RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
			Route:       protocol.TEMP_ROUTE,
		}

		finMess := &protocol.BackWardFin{
			Seq: seq,
		}

		protocol.SendMessage(sMessage, finHeader, finMess, false)
	}()

	mgrTask := &manager.BackwardTask{
		Mode:  manager.B_ADDCONN,
		RPort: backward.Rport,
		Seq:   seq,
	}
	mgr.BackwardManager.TaskChan <- mgrTask
	result := <-mgr.BackwardManager.ResultChan
	mgr.BackwardManager.SeqReady <- true
	if !result.OK {
		share.CloseQuietly(conn)
		return
	}

	// ask for corresponding datachan
	mgrTask = &manager.BackwardTask{
		Mode:  manager.B_GETDATACHAN,
		RPort: backward.Rport,
		Seq:   seq,
	}
	mgr.BackwardManager.TaskChan <- mgrTask
	result = <-mgr.BackwardManager.ResultChan
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
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.BACKWARDDATA,
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

		backwardDataMess := &protocol.BackwardData{
			Seq:     seq,
			DataLen: uint64(length),
			Data:    buffer[:length],
		}

		protocol.SendMessage(sMessage, dataHeader, backwardDataMess, false)
	}
}

func testBackward(mgr *manager.Manager, lPort, rPort string) {
	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	header := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.BACKWARDREADY,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	succMess := &protocol.BackwardReady{
		OK: 1,
	}

	failMess := &protocol.BackwardReady{
		OK: 0,
	}

	listenAddr := fmt.Sprintf("0.0.0.0:%s", rPort)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		protocol.SendMessage(sMessage, header, failMess, false)
		return
	}

	backward := newBackward(listener, lPort, rPort)

	go backward.start(mgr)

	protocol.SendMessage(sMessage, header, succMess, false)
}

func sendDoneMess(all uint16, rPort string) {
	// here is a problem,if some of the backward conns cannot send FIN before DONE,then the FIN they send cannot be processed by admin
	// but it's not a really big problem,because users must know some data maybe lost since they choose to close backward
	sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

	header := &protocol.Header{
		Sender:      tsruntime.Component().UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.BACKWARDSTOPDONE,
		RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
		Route:       protocol.TEMP_ROUTE,
	}

	doneMess := &protocol.BackwardStopDone{
		All:      all,
		UUIDLen:  uint16(len(tsruntime.Component().UUID)),
		UUID:     tsruntime.Component().UUID,
		RPortLen: uint16(len(rPort)),
		RPort:    rPort,
	}

	protocol.SendMessage(sMessage, header, doneMess, false)
}

func DispatchBackwardMess(mgr *manager.Manager) {
	for {
		message := <-mgr.BackwardManager.BackwardMessChan

		switch mess := message.(type) {
		case *protocol.BackwardTest:
			go testBackward(mgr, mess.LPort, mess.RPort)
		case *protocol.BackwardSeq:
			mgrTask := &manager.BackwardTask{
				Mode:  manager.B_GETSEQCHAN,
				RPort: mess.RPort,
				Seq:   mess.Seq,
			}
			mgr.BackwardManager.TaskChan <- mgrTask
			result := <-mgr.BackwardManager.ResultChan

			if result.OK {
				result.SeqChan <- mess.Seq
				<-mgr.BackwardManager.SeqReady
			} else {
				sMessage := protocol.NewUpMsg(tsruntime.Component().Conn, tsruntime.Component().Secret, tsruntime.Component().UUID)

				finHeader := &protocol.Header{
					Sender:      tsruntime.Component().UUID,
					Accepter:    protocol.ADMIN_UUID,
					MessageType: protocol.BACKWARDFIN,
					RouteLen:    uint32(len([]byte(protocol.TEMP_ROUTE))),
					Route:       protocol.TEMP_ROUTE,
				}

				finMess := &protocol.BackWardFin{
					Seq: mess.Seq,
				}

				protocol.ConstructMessage(sMessage, finHeader, finMess, false)
				sMessage.SendMessage()
			}
		case *protocol.BackwardData:
			mgrTask := &manager.BackwardTask{
				Mode: manager.B_GETDATACHAN_WITHOUTUUID,
				Seq:  mess.Seq,
			}
			mgr.BackwardManager.TaskChan <- mgrTask
			result := <-mgr.BackwardManager.ResultChan

			if result.OK {
				if !share.EnqueueBytes(result.DataChan, mess.Data, share.DefaultDataEnqueueTimeout) {
					mgr.BackwardManager.TaskChan <- &manager.BackwardTask{
						Mode: manager.B_CLOSETCP,
						Seq:  mess.Seq,
					}
				}
			}
		case *protocol.BackWardFin:
			mgrTask := &manager.BackwardTask{
				Mode: manager.B_CLOSETCP,
				Seq:  mess.Seq,
			}
			mgr.BackwardManager.TaskChan <- mgrTask
		case *protocol.BackwardStop:
			if mess.All == 1 {
				mgrTask := &manager.BackwardTask{
					Mode: manager.B_CLOSESINGLEALL,
				}
				mgr.BackwardManager.TaskChan <- mgrTask
			} else {
				mgrTask := &manager.BackwardTask{
					Mode:  manager.B_CLOSESINGLE,
					RPort: mess.RPort,
				}
				mgr.BackwardManager.TaskChan <- mgrTask
			}
			<-mgr.BackwardManager.ResultChan
			go sendDoneMess(mess.All, mess.RPort)
		}
	}
}
