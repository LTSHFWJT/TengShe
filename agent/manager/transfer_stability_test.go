package manager

import "testing"

func TestAgentTransferDataChannelSizes(t *testing.T) {
	if forwardDataChanSize < 256 {
		t.Fatalf("forwardDataChanSize = %d, want at least 256", forwardDataChanSize)
	}
	if backwardDataChanSize < 256 {
		t.Fatalf("backwardDataChanSize = %d, want at least 256", backwardDataChanSize)
	}
}

func TestAgentForwardCloseTCPIsIdempotent(t *testing.T) {
	mgr := newForwardManager()
	mgr.forwardStatusMap[42] = &forwardStatus{dataChan: make(chan []byte, forwardDataChanSize)}

	mgr.closeTCP(&ForwardTask{Seq: 42})
	mgr.closeTCP(&ForwardTask{Seq: 42})

	if _, ok := mgr.forwardStatusMap[42]; ok {
		t.Fatal("forward status was not removed")
	}
}

func TestAgentBackwardCloseTCPIsIdempotentAndCleansSeqMap(t *testing.T) {
	mgr := newBackwardManager()
	mgr.backwardSeqMap[42] = "9000"
	mgr.backwardMap["9000"] = &backward{
		backwardStatusMap: map[uint64]*backwardStatus{
			42: {dataChan: make(chan []byte, backwardDataChanSize)},
		},
	}

	mgr.closeTCP(&BackwardTask{Seq: 42})
	mgr.closeTCP(&BackwardTask{Seq: 42})

	if _, ok := mgr.backwardSeqMap[42]; ok {
		t.Fatal("backward seq mapping was not removed")
	}
	if _, ok := mgr.backwardMap["9000"].backwardStatusMap[42]; ok {
		t.Fatal("backward status was not removed")
	}
}
