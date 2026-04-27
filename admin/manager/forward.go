/*
 * @Author: ph4ntom
 * @Date: 2021-04-02 16:01:58
 * @LastEditors: ph4ntom
 * @LastEditTime: 2021-04-02 18:46:53
 */
package manager

import (
	"net"

	"TengShe/share"
)

const (
	F_GETNEWSEQ = iota
	F_NEWFORWARD
	F_ADDCONN
	F_GETDATACHAN
	F_GETDATACHAN_WITHOUTUUID
	F_GETFORWARDINFO
	F_CLOSETCP
	F_CLOSESINGLE
	F_CLOSESINGLEALL
	F_FORCESHUTDOWN
)

const (
	forwardMessageChanSize = 512
	forwardDataChanSize    = 256
)

// forwardManager owns admin-side forward listener/session state.
// Lifecycle: newForward registers a local listener for one node/port,
// getNewSeq allocates per-connection seq values, close paths remove sessions
// or whole listeners, and forceShutdown is used when a node leaves.
type forwardManager struct {
	forwardSeq      uint64
	forwardSeqMap   map[uint64]*fwSeqRelationship  // map[seq](port+uuid), used to find a session owner by seq
	forwardMap      map[string]map[string]*forward // map[uuid]map[port]*forward's detail record forward status
	forwardReadyDel map[int]string                 // map[user option]port, prepared while rendering close choices

	ForwardMessChan chan interface{} // protocol messages dispatched by admin process
	ForwardReady    chan bool        // startup acknowledgement from handler

	TaskChan   chan *ForwardTask   // serialized manager commands
	ResultChan chan *forwardResult // command responses
}

type ForwardTask struct {
	Mode int
	UUID string // node uuid
	Seq  uint64 // seq

	Port        string
	RemoteAddr  string
	CloseTarget int
	Listener    net.Listener
}

type forwardResult struct {
	OK bool

	ForwardSeq  uint64
	DataChan    chan []byte
	ForwardInfo []*forwardInfo
}

type forward struct {
	remoteAddr string
	listener   net.Listener

	forwardStatusMap map[uint64]*forwardStatus
}

type forwardStatus struct {
	dataChan chan []byte
}

type fwSeqRelationship struct {
	uuid string
	port string
}

type forwardInfo struct {
	Seq       int
	Laddr     string
	Raddr     string
	ActiveNum int
}

func newForwardManager() *forwardManager {
	manager := new(forwardManager)

	manager.forwardMap = make(map[string]map[string]*forward)
	manager.forwardSeqMap = make(map[uint64]*fwSeqRelationship)
	manager.ForwardMessChan = make(chan interface{}, forwardMessageChanSize)
	manager.ForwardReady = make(chan bool)

	manager.TaskChan = make(chan *ForwardTask)
	manager.ResultChan = make(chan *forwardResult)

	return manager
}

func (manager *forwardManager) run() {
	for {
		task := <-manager.TaskChan

		switch task.Mode {
		case F_NEWFORWARD:
			manager.newForward(task)
		case F_GETNEWSEQ:
			manager.getNewSeq(task)
		case F_ADDCONN:
			manager.addConn(task)
		case F_GETDATACHAN:
			manager.getDatachan(task)
		case F_GETDATACHAN_WITHOUTUUID:
			manager.getDatachanWithoutUUID(task)
		case F_GETFORWARDINFO:
			manager.getForwardInfo(task)
		case F_CLOSETCP:
			manager.closeTCP(task)
		case F_CLOSESINGLE:
			manager.closeSingle(task)
		case F_CLOSESINGLEALL:
			manager.closeSingleAll(task)
		case F_FORCESHUTDOWN:
			manager.forceShutdown(task)
		}
	}
}

// 2022.7.19 Fix nil pointer bug,thx to @zyylhn
func (manager *forwardManager) newForward(task *ForwardTask) {
	if _, ok := manager.forwardMap[task.UUID]; !ok {
		manager.forwardMap[task.UUID] = make(map[string]*forward)
	}

	manager.forwardMap[task.UUID][task.Port] = new(forward)
	manager.forwardMap[task.UUID][task.Port].listener = task.Listener
	manager.forwardMap[task.UUID][task.Port].remoteAddr = task.RemoteAddr
	manager.forwardMap[task.UUID][task.Port].forwardStatusMap = make(map[uint64]*forwardStatus)

	manager.ResultChan <- &forwardResult{OK: true}
}

func (manager *forwardManager) getNewSeq(task *ForwardTask) {
	manager.forwardSeqMap[manager.forwardSeq] = &fwSeqRelationship{uuid: task.UUID, port: task.Port}
	manager.ResultChan <- &forwardResult{ForwardSeq: manager.forwardSeq}
	manager.forwardSeq++
}

func (manager *forwardManager) addConn(task *ForwardTask) {
	if _, ok := manager.forwardSeqMap[task.Seq]; !ok {
		manager.ResultChan <- &forwardResult{OK: false}
		return
	}

	nodeForward, ok := manager.forwardMap[task.UUID]
	if !ok {
		manager.ResultChan <- &forwardResult{OK: false}
		return
	}
	portForward, ok := nodeForward[task.Port]
	if !ok {
		manager.ResultChan <- &forwardResult{OK: false}
		return
	}

	portForward.forwardStatusMap[task.Seq] = new(forwardStatus)
	portForward.forwardStatusMap[task.Seq].dataChan = make(chan []byte, forwardDataChanSize)
	manager.ResultChan <- &forwardResult{OK: true}
}

func (manager *forwardManager) getDatachan(task *ForwardTask) {
	if _, ok := manager.forwardSeqMap[task.Seq]; !ok {
		manager.ResultChan <- &forwardResult{OK: false}
		return
	}

	nodeForward, ok := manager.forwardMap[task.UUID]
	if !ok {
		manager.ResultChan <- &forwardResult{OK: false}
		return
	}
	portForward, ok := nodeForward[task.Port]
	if !ok {
		manager.ResultChan <- &forwardResult{OK: false}
		return
	}

	if status, ok := portForward.forwardStatusMap[task.Seq]; ok && status != nil { // need to check ,because you will never know when fin come
		manager.ResultChan <- &forwardResult{
			OK:       true,
			DataChan: status.dataChan,
		}
	} else {
		manager.ResultChan <- &forwardResult{OK: false}
	}
}

func (manager *forwardManager) getDatachanWithoutUUID(task *ForwardTask) {
	if _, ok := manager.forwardSeqMap[task.Seq]; !ok {
		manager.ResultChan <- &forwardResult{OK: false}
		return
	}

	uuid := manager.forwardSeqMap[task.Seq].uuid
	port := manager.forwardSeqMap[task.Seq].port

	nodeForward, ok := manager.forwardMap[uuid]
	if !ok {
		manager.ResultChan <- &forwardResult{OK: false}
		return
	}
	portForward, ok := nodeForward[port]
	if !ok {
		manager.ResultChan <- &forwardResult{OK: false}
		return
	}
	status, ok := portForward.forwardStatusMap[task.Seq]
	if !ok || status == nil {
		manager.ResultChan <- &forwardResult{OK: false}
		return
	}

	manager.ResultChan <- &forwardResult{
		OK:       true,
		DataChan: status.dataChan,
	}
}

func (manager *forwardManager) getForwardInfo(task *ForwardTask) {
	manager.forwardReadyDel = make(map[int]string)

	var result []*forwardInfo
	seq := 1

	if _, ok := manager.forwardMap[task.UUID]; ok {
		for port, info := range manager.forwardMap[task.UUID] {
			manager.forwardReadyDel[seq] = port
			result = append(result, &forwardInfo{Seq: seq, Laddr: info.listener.Addr().String(), Raddr: info.remoteAddr, ActiveNum: len(info.forwardStatusMap)})
			seq++
		}
		manager.ResultChan <- &forwardResult{
			OK:          true,
			ForwardInfo: result,
		}
	} else {
		manager.ResultChan <- &forwardResult{
			OK:          false,
			ForwardInfo: result,
		}
	}
}

func (manager *forwardManager) closeTCP(task *ForwardTask) {
	if _, ok := manager.forwardSeqMap[task.Seq]; !ok {
		return
	}

	uuid := manager.forwardSeqMap[task.Seq].uuid
	port := manager.forwardSeqMap[task.Seq].port

	nodeForward, ok := manager.forwardMap[uuid]
	if !ok {
		delete(manager.forwardSeqMap, task.Seq)
		return
	}
	portForward, ok := nodeForward[port]
	if !ok {
		delete(manager.forwardSeqMap, task.Seq)
		return
	}
	status, ok := portForward.forwardStatusMap[task.Seq]
	if !ok {
		delete(manager.forwardSeqMap, task.Seq)
		return
	}

	share.CloseBytesChanQuietly(status.dataChan)

	delete(portForward.forwardStatusMap, task.Seq)
	delete(manager.forwardSeqMap, task.Seq)
}

func (manager *forwardManager) closeSingle(task *ForwardTask) {
	// find port that user want to del
	port := manager.forwardReadyDel[task.CloseTarget]
	// close corresponding listener
	share.CloseQuietly(manager.forwardMap[task.UUID][port].listener)
	// clear every single connection's resources
	for seq, status := range manager.forwardMap[task.UUID][port].forwardStatusMap {
		share.CloseBytesChanQuietly(status.dataChan)
		delete(manager.forwardMap[task.UUID][port].forwardStatusMap, seq)
	}
	// delete the target port
	delete(manager.forwardMap[task.UUID], port)
	// clear the seqmap that match relationship.uuid == task.UUID && relationship.port == port
	for seq, relationship := range manager.forwardSeqMap {
		if relationship.uuid == task.UUID && relationship.port == port {
			delete(manager.forwardSeqMap, seq)
		}
	}
	// if no other forward services running on current node,delete node from manager.forwardMap
	if len(manager.forwardMap[task.UUID]) == 0 {
		delete(manager.forwardMap, task.UUID)
	}

	manager.ResultChan <- &forwardResult{OK: true}
}

func (manager *forwardManager) closeSingleAll(task *ForwardTask) {
	for port, forward := range manager.forwardMap[task.UUID] {
		share.CloseQuietly(forward.listener)

		for seq, status := range forward.forwardStatusMap {
			share.CloseBytesChanQuietly(status.dataChan)
			delete(forward.forwardStatusMap, seq)
		}

		delete(manager.forwardMap[task.UUID], port)
	}

	for seq, relationship := range manager.forwardSeqMap {
		if relationship.uuid == task.UUID {
			delete(manager.forwardSeqMap, seq)
		}
	}

	delete(manager.forwardMap, task.UUID)

	manager.ResultChan <- &forwardResult{OK: true}
}

func (manager *forwardManager) forceShutdown(task *ForwardTask) {
	if _, ok := manager.forwardMap[task.UUID]; ok {
		manager.closeSingleAll(task)
	} else {
		manager.ResultChan <- &forwardResult{OK: true}
	}
}
