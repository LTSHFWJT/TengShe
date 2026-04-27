/*
 * @Author: ph4ntom
 * @Date: 2021-04-02 15:43:04
 * @LastEditors: ph4ntom
 * @LastEditTime: 2021-04-02 17:29:12
 */
package manager

import (
	"net"

	"TengShe/share"
)

const (
	S_NEWSOCKS = iota
	S_ADDTCPSOCKET
	S_GETNEWSEQ
	S_GETTCPDATACHAN
	S_GETUDPDATACHAN
	S_GETTCPDATACHAN_WITHOUTUUID
	S_GETUDPDATACHAN_WITHOUTUUID
	S_CLOSETCP
	S_GETUDPSTARTINFO
	S_UPDATEUDP
	S_GETSOCKSINFO
	S_CLOSESOCKS
	S_FORCESHUTDOWN
)

const (
	socksMessageChanSize = 512
	socksTCPDataChanSize = 256
	socksUDPDataChanSize = 256
)

// socksManager owns admin-side SOCKS listener/session state.
// Lifecycle: newSocks registers one SOCKS service for a node, per-connection
// TCP/UDP sessions are keyed by seq, and close/force-shutdown paths release
// listeners, sockets, and data channels.
type socksManager struct {
	socksSeq    uint64
	socksSeqMap map[uint64]string // map[seq]uuid, used to find the owner node by session seq
	socksMap    map[string]*socks // map[uuid]SOCKS service detail and active sessions

	SocksMessChan chan interface{} // protocol messages dispatched by admin process
	SocksReady    chan bool        // startup acknowledgement from handler

	TaskChan   chan *SocksTask   // serialized manager commands
	ResultChan chan *socksResult // command responses
}

type SocksTask struct {
	Mode int
	UUID string // node uuid
	Seq  uint64 // seq

	SocksAddr        string
	SocksPort        string
	SocksUsername    string
	SocksPassword    string
	SocksTCPListener net.Listener
	SocksTCPSocket   net.Conn
	SocksUDPListener *net.UDPConn
}

type socksResult struct {
	OK   bool
	UUID string

	SocksSeq    uint64
	TCPAddr     string
	SocksInfo   *socksInfo
	TCPDataChan chan []byte
	UDPDataChan chan []byte
}

type socks struct {
	addr     string
	port     string
	username string
	password string
	listener net.Listener

	socksStatusMap map[uint64]*socksStatus
}

type socksStatus struct {
	isUDP bool
	tcp   *tcpSocks
	udp   *udpSocks
}

type socksInfo struct {
	Addr     string
	Port     string
	Username string
	Password string
}

type tcpSocks struct {
	dataChan chan []byte
	conn     net.Conn
}

type udpSocks struct {
	dataChan chan []byte
	listener *net.UDPConn
}

func newSocksManager() *socksManager {
	manager := new(socksManager)

	manager.socksMap = make(map[string]*socks)
	manager.socksSeqMap = make(map[uint64]string)
	manager.SocksMessChan = make(chan interface{}, socksMessageChanSize)
	manager.SocksReady = make(chan bool)

	manager.TaskChan = make(chan *SocksTask)
	manager.ResultChan = make(chan *socksResult)

	return manager
}

func (manager *socksManager) run() {
	for {
		task := <-manager.TaskChan

		switch task.Mode {
		case S_NEWSOCKS:
			manager.newSocks(task)
		case S_ADDTCPSOCKET:
			manager.addSocksTCPSocket(task)
		case S_GETNEWSEQ:
			manager.getSocksSeq(task)
		case S_GETTCPDATACHAN:
			manager.getTCPDataChan(task)
		case S_GETUDPDATACHAN:
			manager.getUDPDataChan(task)
		case S_GETTCPDATACHAN_WITHOUTUUID:
			manager.getTCPDataChanWithoutUUID(task)
		case S_GETUDPDATACHAN_WITHOUTUUID:
			manager.getUDPDataChanWithoutUUID(task)
		case S_CLOSETCP:
			manager.closeTCP(task)
		case S_GETUDPSTARTINFO:
			manager.getUDPStartInfo(task)
		case S_UPDATEUDP:
			manager.updateUDP(task)
		case S_GETSOCKSINFO:
			manager.getSocksInfo(task)
		case S_CLOSESOCKS:
			manager.closeSocks(task)
		case S_FORCESHUTDOWN:
			manager.forceShutdown(task)
		}
	}
}

func (manager *socksManager) newSocks(task *SocksTask) {
	if _, ok := manager.socksMap[task.UUID]; !ok {
		manager.socksMap[task.UUID] = new(socks)
		manager.socksMap[task.UUID].addr = task.SocksAddr
		manager.socksMap[task.UUID].port = task.SocksPort
		manager.socksMap[task.UUID].username = task.SocksUsername
		manager.socksMap[task.UUID].password = task.SocksPassword
		manager.socksMap[task.UUID].socksStatusMap = make(map[uint64]*socksStatus)
		manager.socksMap[task.UUID].listener = task.SocksTCPListener
		manager.ResultChan <- &socksResult{OK: true}
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

func (manager *socksManager) addSocksTCPSocket(task *SocksTask) {
	if _, ok := manager.socksMap[task.UUID]; ok {
		manager.socksMap[task.UUID].socksStatusMap[task.Seq] = new(socksStatus)
		manager.socksMap[task.UUID].socksStatusMap[task.Seq].tcp = new(tcpSocks) // no need to check if socksStatusMap[task.Seq] exist,because it must exist
		manager.socksMap[task.UUID].socksStatusMap[task.Seq].tcp.dataChan = make(chan []byte, socksTCPDataChanSize)
		manager.socksMap[task.UUID].socksStatusMap[task.Seq].tcp.conn = task.SocksTCPSocket
		manager.ResultChan <- &socksResult{OK: true}
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

func (manager *socksManager) getSocksSeq(task *SocksTask) {
	// Use seqmap to record the UUIDNum <-> Seq relationship to make search quicker
	manager.socksSeqMap[manager.socksSeq] = task.UUID
	manager.ResultChan <- &socksResult{SocksSeq: manager.socksSeq}
	manager.socksSeq++
}

func (manager *socksManager) getTCPDataChan(task *SocksTask) {
	if socksInfo, ok := manager.socksMap[task.UUID]; ok {
		status, ok := socksInfo.socksStatusMap[task.Seq]
		if !ok || status.tcp == nil {
			manager.ResultChan <- &socksResult{OK: false}
			return
		}
		manager.ResultChan <- &socksResult{
			OK:          true,
			TCPDataChan: status.tcp.dataChan,
		}
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

func (manager *socksManager) getUDPDataChan(task *SocksTask) {
	if socksInfo, ok := manager.socksMap[task.UUID]; ok {
		if status, ok := socksInfo.socksStatusMap[task.Seq]; ok && status.udp != nil {
			manager.ResultChan <- &socksResult{
				OK:          true,
				UDPDataChan: status.udp.dataChan,
			}
		} else {
			manager.ResultChan <- &socksResult{OK: false}
		}
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

func (manager *socksManager) getTCPDataChanWithoutUUID(task *SocksTask) {
	if _, ok := manager.socksSeqMap[task.Seq]; !ok {
		manager.ResultChan <- &socksResult{OK: false}
		return
	}

	uuid := manager.socksSeqMap[task.Seq]
	// if "manager.socksSeqMap[task.Seq]" exist, "manager.socksMap[uuid]" must exist too
	socksInfo, ok := manager.socksMap[uuid]
	if !ok {
		manager.ResultChan <- &socksResult{OK: false}
		return
	}

	if status, ok := socksInfo.socksStatusMap[task.Seq]; ok && status.tcp != nil {
		manager.ResultChan <- &socksResult{
			OK:          true,
			TCPDataChan: status.tcp.dataChan,
		}
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

func (manager *socksManager) getUDPDataChanWithoutUUID(task *SocksTask) {
	if _, ok := manager.socksSeqMap[task.Seq]; !ok {
		manager.ResultChan <- &socksResult{OK: false}
		return
	}

	uuid := manager.socksSeqMap[task.Seq]
	// manager.socksMap[uuid] must exist if manager.socksSeqMap[task.Seq] exist
	socksInfo, ok := manager.socksMap[uuid]
	if !ok {
		manager.ResultChan <- &socksResult{OK: false}
		return
	}

	if status, ok := socksInfo.socksStatusMap[task.Seq]; ok && status.udp != nil {
		manager.ResultChan <- &socksResult{
			OK:          true,
			UDPDataChan: status.udp.dataChan,
		}
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

// close TCP include close UDP,cuz UDP's control channel is TCP,if TCP broken,UDP is also forced to be shut down
func (manager *socksManager) closeTCP(task *SocksTask) {
	uuid, ok := manager.socksSeqMap[task.Seq]
	if !ok {
		return
	}

	socksInfo, ok := manager.socksMap[uuid]
	if !ok {
		delete(manager.socksSeqMap, task.Seq)
		return
	}

	status, ok := socksInfo.socksStatusMap[task.Seq]
	if !ok {
		delete(manager.socksSeqMap, task.Seq)
		return
	}

	// bugfix: In order to avoid data loss,so not close conn&listener here.Thx to @lz520520
	if status.tcp != nil {
		closeBytesChanQuietly(status.tcp.dataChan)
	}

	if status.isUDP && status.udp != nil {
		closeBytesChanQuietly(status.udp.dataChan)
	}

	delete(socksInfo.socksStatusMap, task.Seq)
	delete(manager.socksSeqMap, task.Seq)
}

func (manager *socksManager) getUDPStartInfo(task *SocksTask) {
	if _, ok := manager.socksSeqMap[task.Seq]; !ok {
		manager.ResultChan <- &socksResult{OK: false}
		return
	}

	uuid := manager.socksSeqMap[task.Seq]
	socksInfo, ok := manager.socksMap[uuid]
	if !ok {
		manager.ResultChan <- &socksResult{OK: false}
		return
	}

	if status, ok := socksInfo.socksStatusMap[task.Seq]; ok && status.tcp != nil && status.tcp.conn != nil {
		tcpAddr, ok := status.tcp.conn.LocalAddr().(*net.TCPAddr)
		if !ok {
			manager.ResultChan <- &socksResult{OK: false}
			return
		}
		manager.ResultChan <- &socksResult{
			OK:      true,
			TCPAddr: tcpAddr.IP.String(),
			UUID:    uuid,
		}
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

func (manager *socksManager) updateUDP(task *SocksTask) {
	if _, ok := manager.socksMap[task.UUID]; ok {
		if _, ok := manager.socksMap[task.UUID].socksStatusMap[task.Seq]; ok {
			manager.socksMap[task.UUID].socksStatusMap[task.Seq].isUDP = true
			manager.socksMap[task.UUID].socksStatusMap[task.Seq].udp = new(udpSocks)
			manager.socksMap[task.UUID].socksStatusMap[task.Seq].udp.dataChan = make(chan []byte, socksUDPDataChanSize)
			manager.socksMap[task.UUID].socksStatusMap[task.Seq].udp.listener = task.SocksUDPListener
			manager.ResultChan <- &socksResult{OK: true}
		} else {
			manager.ResultChan <- &socksResult{OK: false}
		}
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

func (manager *socksManager) getSocksInfo(task *SocksTask) {
	if _, ok := manager.socksMap[task.UUID]; ok {
		manager.ResultChan <- &socksResult{
			OK: true,
			SocksInfo: &socksInfo{
				Addr:     manager.socksMap[task.UUID].addr,
				Port:     manager.socksMap[task.UUID].port,
				Username: manager.socksMap[task.UUID].username,
				Password: manager.socksMap[task.UUID].password,
			},
		}
	} else {
		manager.ResultChan <- &socksResult{
			OK:        false,
			SocksInfo: &socksInfo{},
		}
	}
}

func (manager *socksManager) closeSocks(task *SocksTask) {
	socksInfo, ok := manager.socksMap[task.UUID]
	if !ok {
		manager.ResultChan <- &socksResult{OK: true}
		return
	}

	share.CloseQuietly(socksInfo.listener)
	for seq, status := range socksInfo.socksStatusMap {
		// bugfix: In order to avoid data loss,so not close conn&listener here.Thx to @lz520520
		if status.tcp != nil {
			closeBytesChanQuietly(status.tcp.dataChan)
		}
		if status.isUDP && status.udp != nil {
			closeBytesChanQuietly(status.udp.dataChan)
		}
		delete(socksInfo.socksStatusMap, seq)
	}

	for seq, uuid := range manager.socksSeqMap {
		if uuid == task.UUID {
			delete(manager.socksSeqMap, seq)
		}
	}

	delete(manager.socksMap, task.UUID) // we delete corresponding "socksMap"
	manager.ResultChan <- &socksResult{OK: true}
}

func (manager *socksManager) forceShutdown(task *SocksTask) {
	if _, ok := manager.socksMap[task.UUID]; ok {
		manager.closeSocks(task)
	} else {
		manager.ResultChan <- &socksResult{OK: true}
	}
}

func closeBytesChanQuietly(ch chan []byte) {
	defer func() { _ = recover() }()
	if ch != nil {
		close(ch)
	}
}
