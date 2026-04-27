package manager

const (
	S_CHECKTCP = iota
	S_CHECKUDP
	S_UPDATEUDPHEADER
	S_GETTCPDATACHAN
	S_GETUDPCHANS
	S_GETUDPHEADER
	S_CLOSETCP
	S_CHECKSOCKSREADY
	S_FORCESHUTDOWN
)

const (
	socksMessageChanSize = 512
	socksTCPDataChanSize = 256
	socksUDPDataChanSize = 256
)

// socksManager owns agent-side SOCKS relay session state.
// Lifecycle: SOCKSSTART/SOCKSTCPDATA/SOCKSUDPDATA create or update sessions
// keyed by seq, FIN and force-shutdown close data channels and remove state.
type socksManager struct {
	socksStatusMap map[uint64]*socksStatus
	SocksMessChan  chan interface{} // protocol messages dispatched by agent process

	TaskChan   chan *SocksTask   // serialized manager commands
	ResultChan chan *socksResult // command responses
}

type SocksTask struct {
	Mode int
	Seq  uint64

	SocksHeaderAddr string
	SocksHeader     []byte
}

type socksResult struct {
	OK bool

	SocksSeqExist  bool
	DataChan       chan []byte
	ReadyChan      chan string
	SocksID        uint64
	SocksUDPHeader []byte
}

type socksStatus struct {
	isUDP bool
	tcp   *tcpSocks
	udp   *udpSocks
}

type tcpSocks struct {
	dataChan chan []byte
}

type udpSocks struct {
	dataChan    chan []byte
	readyChan   chan string
	headerPairs map[string][]byte
}

func newSocksManager() *socksManager {
	manager := new(socksManager)

	manager.socksStatusMap = make(map[uint64]*socksStatus)
	manager.SocksMessChan = make(chan interface{}, socksMessageChanSize)

	manager.ResultChan = make(chan *socksResult)
	manager.TaskChan = make(chan *SocksTask)

	return manager
}

func (manager *socksManager) run() {
	for {
		task := <-manager.TaskChan

		switch task.Mode {
		case S_GETTCPDATACHAN:
			manager.getTCPDataChan(task)
		case S_GETUDPCHANS:
			manager.getUDPChans(task)
		case S_GETUDPHEADER:
			manager.getUDPHeader(task)
		case S_CHECKTCP:
			manager.checkTCP(task)
		case S_CHECKUDP:
			manager.checkUDP(task)
		case S_UPDATEUDPHEADER:
			manager.updateUDPHeader(task)
		case S_CLOSETCP:
			manager.closeTCP(task)
		case S_CHECKSOCKSREADY:
			manager.checkSocksReady()
		case S_FORCESHUTDOWN:
			manager.forceShutdown()
		}
	}
}

func (manager *socksManager) getTCPDataChan(task *SocksTask) {
	if status, ok := manager.socksStatusMap[task.Seq]; ok && status.tcp != nil {
		manager.ResultChan <- &socksResult{
			SocksSeqExist: true,
			DataChan:      status.tcp.dataChan,
		}
	} else {
		manager.socksStatusMap[task.Seq] = new(socksStatus)
		manager.socksStatusMap[task.Seq].tcp = new(tcpSocks)
		manager.socksStatusMap[task.Seq].tcp.dataChan = make(chan []byte, socksTCPDataChanSize) // register it!
		manager.ResultChan <- &socksResult{
			SocksSeqExist: false,
			DataChan:      manager.socksStatusMap[task.Seq].tcp.dataChan,
		} // tell upstream result
	}
}

func (manager *socksManager) getUDPChans(task *SocksTask) {
	if status, ok := manager.socksStatusMap[task.Seq]; ok && status.udp != nil {
		manager.ResultChan <- &socksResult{
			OK:        true,
			DataChan:  status.udp.dataChan,
			ReadyChan: status.udp.readyChan,
		}
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

func (manager *socksManager) checkTCP(task *SocksTask) {
	if _, ok := manager.socksStatusMap[task.Seq]; ok {
		manager.ResultChan <- &socksResult{OK: true}
	} else {
		manager.ResultChan <- &socksResult{OK: false} // avoid the scenario that admin conn ask to fin before "socks.buildConn()" call "updateTCP()"
	}
}

func (manager *socksManager) checkUDP(task *SocksTask) {
	if _, ok := manager.socksStatusMap[task.Seq]; ok {
		manager.socksStatusMap[task.Seq].isUDP = true
		manager.socksStatusMap[task.Seq].udp = new(udpSocks)
		manager.socksStatusMap[task.Seq].udp.dataChan = make(chan []byte, socksUDPDataChanSize)
		manager.socksStatusMap[task.Seq].udp.readyChan = make(chan string)
		manager.socksStatusMap[task.Seq].udp.headerPairs = make(map[string][]byte)
		manager.ResultChan <- &socksResult{OK: true} // tell upstream work done
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

func (manager *socksManager) updateUDPHeader(task *SocksTask) {
	if status, ok := manager.socksStatusMap[task.Seq]; ok && status.udp != nil {
		status.udp.headerPairs[task.SocksHeaderAddr] = task.SocksHeader
	}
	manager.ResultChan <- &socksResult{}
}

func (manager *socksManager) getUDPHeader(task *SocksTask) {
	if status, ok := manager.socksStatusMap[task.Seq]; ok && status.udp != nil {
		if _, ok := status.udp.headerPairs[task.SocksHeaderAddr]; ok {
			manager.ResultChan <- &socksResult{
				OK:             true,
				SocksUDPHeader: status.udp.headerPairs[task.SocksHeaderAddr],
			}
		} else {
			manager.ResultChan <- &socksResult{OK: false}
		}
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

func (manager *socksManager) closeTCP(task *SocksTask) {
	status, ok := manager.socksStatusMap[task.Seq]
	if !ok {
		return
	}

	if status.tcp != nil {
		closeBytesChanQuietly(status.tcp.dataChan)
	}

	if status.isUDP && status.udp != nil {
		closeBytesChanQuietly(status.udp.dataChan)
		closeStringChanQuietly(status.udp.readyChan)
		status.udp.headerPairs = nil
	}

	delete(manager.socksStatusMap, task.Seq) // upstream not waiting
}

func (manager *socksManager) checkSocksReady() {
	if len(manager.socksStatusMap) == 0 {
		manager.ResultChan <- &socksResult{OK: true}
	} else {
		manager.ResultChan <- &socksResult{OK: false}
	}
}

func (manager *socksManager) forceShutdown() {
	for seq, status := range manager.socksStatusMap {
		if status.tcp != nil {
			closeBytesChanQuietly(status.tcp.dataChan)
		}

		if status.isUDP && status.udp != nil {
			closeBytesChanQuietly(status.udp.dataChan)
			closeStringChanQuietly(status.udp.readyChan)
			status.udp.headerPairs = nil
		}

		delete(manager.socksStatusMap, seq)
	}

	manager.ResultChan <- &socksResult{OK: true}
}

func closeBytesChanQuietly(ch chan []byte) {
	defer func() { _ = recover() }()
	if ch != nil {
		close(ch)
	}
}

func closeStringChanQuietly(ch chan string) {
	defer func() { _ = recover() }()
	if ch != nil {
		close(ch)
	}
}
