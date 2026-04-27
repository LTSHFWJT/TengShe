package manager

import "TengShe/share"

const (
	F_NEWFORWARD = iota
	F_GETDATACHAN
	F_CHECKFORWARD
	F_CLOSETCP
	F_FORCESHUTDOWN
)

const (
	forwardMessageChanSize = 512
	forwardDataChanSize    = 256
)

// forwardManager owns agent-side forward connection state.
// Lifecycle: FORWARDSTART registers a seq, data messages use its channel,
// and FIN/offline paths close the channel and remove the seq record.
type forwardManager struct {
	forwardStatusMap map[uint64]*forwardStatus
	ForwardMessChan  chan interface{} // protocol messages dispatched by agent process

	TaskChan   chan *ForwardTask   // serialized manager commands
	ResultChan chan *forwardResult // command responses
}

type ForwardTask struct {
	Mode int
	Seq  uint64
}

type forwardResult struct {
	OK bool

	DataChan chan []byte
}

type forwardStatus struct {
	dataChan chan []byte
}

func newForwardManager() *forwardManager {
	manager := new(forwardManager)

	manager.forwardStatusMap = make(map[uint64]*forwardStatus)
	manager.ForwardMessChan = make(chan interface{}, forwardMessageChanSize)

	manager.ResultChan = make(chan *forwardResult)
	manager.TaskChan = make(chan *ForwardTask)

	return manager
}

func (manager *forwardManager) run() {
	for {
		task := <-manager.TaskChan

		switch task.Mode {
		case F_NEWFORWARD:
			manager.newForward(task)
		case F_GETDATACHAN:
			manager.getDataChan(task)
		case F_CHECKFORWARD:
			manager.checkForward(task)
		case F_CLOSETCP:
			manager.closeTCP(task)
		case F_FORCESHUTDOWN:
			manager.forceShutdown()
		}
	}
}

func (manager *forwardManager) newForward(task *ForwardTask) {
	manager.forwardStatusMap[task.Seq] = new(forwardStatus)
	manager.forwardStatusMap[task.Seq].dataChan = make(chan []byte, forwardDataChanSize)
	manager.ResultChan <- &forwardResult{OK: true}
}

func (manager *forwardManager) checkForward(task *ForwardTask) {
	if _, ok := manager.forwardStatusMap[task.Seq]; ok {
		manager.ResultChan <- &forwardResult{OK: true}
	} else {
		manager.ResultChan <- &forwardResult{OK: false}
	}
}

func (manager *forwardManager) getDataChan(task *ForwardTask) {
	if status, ok := manager.forwardStatusMap[task.Seq]; ok && status != nil {
		manager.ResultChan <- &forwardResult{
			OK:       true,
			DataChan: status.dataChan,
		}
	} else {
		manager.ResultChan <- &forwardResult{OK: false}
	}
}

func (manager *forwardManager) closeTCP(task *ForwardTask) {
	status, ok := manager.forwardStatusMap[task.Seq]
	if !ok {
		return
	}

	share.CloseBytesChanQuietly(status.dataChan)

	delete(manager.forwardStatusMap, task.Seq)
}

func (manager *forwardManager) forceShutdown() {
	for seq, status := range manager.forwardStatusMap {
		share.CloseBytesChanQuietly(status.dataChan)
		delete(manager.forwardStatusMap, seq)
	}

	manager.ResultChan <- &forwardResult{OK: true}
}
