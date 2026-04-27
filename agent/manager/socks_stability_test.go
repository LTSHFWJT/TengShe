package manager

import "testing"

func TestAgentSocksDataChannelSizes(t *testing.T) {
	if socksTCPDataChanSize < 256 {
		t.Fatalf("socksTCPDataChanSize = %d, want at least 256", socksTCPDataChanSize)
	}
	if socksUDPDataChanSize < 256 {
		t.Fatalf("socksUDPDataChanSize = %d, want at least 256", socksUDPDataChanSize)
	}
}

func TestAgentSocksCloseTCPIsIdempotent(t *testing.T) {
	mgr := newSocksManager()
	mgr.socksStatusMap[42] = &socksStatus{
		isUDP: true,
		tcp: &tcpSocks{
			dataChan: make(chan []byte, socksTCPDataChanSize),
		},
		udp: &udpSocks{
			dataChan:    make(chan []byte, socksUDPDataChanSize),
			readyChan:   make(chan string),
			headerPairs: map[string][]byte{"127.0.0.1:53": []byte("header")},
		},
	}

	mgr.closeTCP(&SocksTask{Seq: 42})
	mgr.closeTCP(&SocksTask{Seq: 42})

	if _, ok := mgr.socksStatusMap[42]; ok {
		t.Fatal("socks status was not removed")
	}
}
