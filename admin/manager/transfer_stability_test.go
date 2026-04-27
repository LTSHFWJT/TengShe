package manager

import "testing"

func TestAdminTransferDataChannelSizes(t *testing.T) {
	if forwardDataChanSize < 256 {
		t.Fatalf("forwardDataChanSize = %d, want at least 256", forwardDataChanSize)
	}
	if backwardDataChanSize < 256 {
		t.Fatalf("backwardDataChanSize = %d, want at least 256", backwardDataChanSize)
	}
}

func TestAdminForwardCloseTCPIsIdempotentAndCleansSeqMap(t *testing.T) {
	mgr := newForwardManager()
	mgr.forwardSeqMap[42] = &fwSeqRelationship{uuid: "node-1", port: "8080"}
	mgr.forwardMap["node-1"] = map[string]*forward{
		"8080": {
			forwardStatusMap: map[uint64]*forwardStatus{
				42: {dataChan: make(chan []byte, forwardDataChanSize)},
			},
		},
	}

	mgr.closeTCP(&ForwardTask{Seq: 42})
	mgr.closeTCP(&ForwardTask{Seq: 42})

	if _, ok := mgr.forwardSeqMap[42]; ok {
		t.Fatal("forward seq mapping was not removed")
	}
	if _, ok := mgr.forwardMap["node-1"]["8080"].forwardStatusMap[42]; ok {
		t.Fatal("forward status was not removed")
	}
}

func TestAdminBackwardCloseTCPIsIdempotentAndCleansSeqMap(t *testing.T) {
	mgr := newBackwardManager()
	mgr.backwardSeqMap[42] = &bwSeqRelationship{uuid: "node-1", rPort: "9000"}
	mgr.backwardMap["node-1"] = map[string]*backward{
		"9000": {
			backwardStatusMap: map[uint64]*backwardStatus{
				42: {dataChan: make(chan []byte, backwardDataChanSize)},
			},
		},
	}

	mgr.closeTCP(&BackwardTask{Seq: 42})
	mgr.closeTCP(&BackwardTask{Seq: 42})

	if _, ok := mgr.backwardSeqMap[42]; ok {
		t.Fatal("backward seq mapping was not removed")
	}
	if _, ok := mgr.backwardMap["node-1"]["9000"].backwardStatusMap[42]; ok {
		t.Fatal("backward status was not removed")
	}
}
