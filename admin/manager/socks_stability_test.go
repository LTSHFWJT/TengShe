package manager

import (
	"net"
	"testing"

	"TengShe/share"
)

func TestAdminSocksDataChannelSizes(t *testing.T) {
	if socksTCPDataChanSize < 256 {
		t.Fatalf("socksTCPDataChanSize = %d, want at least 256", socksTCPDataChanSize)
	}
	if socksUDPDataChanSize < 256 {
		t.Fatalf("socksUDPDataChanSize = %d, want at least 256", socksUDPDataChanSize)
	}
}

func TestAdminSocksCloseTCPIsIdempotentAndCleansSeqMap(t *testing.T) {
	client, server := net.Pipe()
	defer share.CloseQuietly(client)
	defer share.CloseQuietly(server)

	mgr := newSocksManager()
	mgr.socksSeqMap[42] = "node-1"
	mgr.socksMap["node-1"] = &socks{
		socksStatusMap: map[uint64]*socksStatus{
			42: {
				isUDP: true,
				tcp: &tcpSocks{
					dataChan: make(chan []byte, socksTCPDataChanSize),
					conn:     client,
				},
				udp: &udpSocks{
					dataChan: make(chan []byte, socksUDPDataChanSize),
				},
			},
		},
	}

	mgr.closeTCP(&SocksTask{Seq: 42})
	mgr.closeTCP(&SocksTask{Seq: 42})

	if _, ok := mgr.socksSeqMap[42]; ok {
		t.Fatal("seq mapping was not removed")
	}
	if _, ok := mgr.socksMap["node-1"].socksStatusMap[42]; ok {
		t.Fatal("socks status was not removed")
	}
}
