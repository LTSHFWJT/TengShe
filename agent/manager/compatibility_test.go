package manager

import (
	"testing"

	"TengShe/share"
)

func TestAgentManagerTaskModeValuesRemainCompatible(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"S_CHECKTCP", S_CHECKTCP, 0},
		{"S_CHECKUDP", S_CHECKUDP, 1},
		{"S_UPDATEUDPHEADER", S_UPDATEUDPHEADER, 2},
		{"S_GETTCPDATACHAN", S_GETTCPDATACHAN, 3},
		{"S_GETUDPCHANS", S_GETUDPCHANS, 4},
		{"S_GETUDPHEADER", S_GETUDPHEADER, 5},
		{"S_CLOSETCP", S_CLOSETCP, 6},
		{"S_CHECKSOCKSREADY", S_CHECKSOCKSREADY, 7},
		{"S_FORCESHUTDOWN", S_FORCESHUTDOWN, 8},
		{"C_NEWCHILD", C_NEWCHILD, 0},
		{"C_GETCONN", C_GETCONN, 1},
		{"C_GETALLCHILDREN", C_GETALLCHILDREN, 2},
		{"C_DELCHILD", C_DELCHILD, 3},
		{"F_NEWFORWARD", F_NEWFORWARD, 0},
		{"F_GETDATACHAN", F_GETDATACHAN, 1},
		{"F_CHECKFORWARD", F_CHECKFORWARD, 2},
		{"F_CLOSETCP", F_CLOSETCP, 3},
		{"F_FORCESHUTDOWN", F_FORCESHUTDOWN, 4},
		{"B_NEWBACKWARD", B_NEWBACKWARD, 0},
		{"B_GETSEQCHAN", B_GETSEQCHAN, 1},
		{"B_ADDCONN", B_ADDCONN, 2},
		{"B_GETDATACHAN", B_GETDATACHAN, 3},
		{"B_GETDATACHAN_WITHOUTUUID", B_GETDATACHAN_WITHOUTUUID, 4},
		{"B_CLOSETCP", B_CLOSETCP, 5},
		{"B_CLOSESINGLE", B_CLOSESINGLE, 6},
		{"B_CLOSESINGLEALL", B_CLOSESINGLEALL, 7},
		{"B_FORCESHUTDOWN", B_FORCESHUTDOWN, 8},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Fatalf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func TestAgentManagerChannelCompatibility(t *testing.T) {
	mgr := NewManager(share.NewFile())

	tests := []struct {
		name string
		cap  int
		want int
	}{
		{"SocksMessChan", cap(mgr.SocksManager.SocksMessChan), socksMessageChanSize},
		{"ForwardMessChan", cap(mgr.ForwardManager.ForwardMessChan), forwardMessageChanSize},
		{"BackwardMessChan", cap(mgr.BackwardManager.BackwardMessChan), backwardMessageChanSize},
		{"FileMessChan", cap(mgr.FileManager.FileMessChan), 5},
		{"SSHMessChan", cap(mgr.SSHManager.SSHMessChan), 5},
		{"SSHTunnelMessChan", cap(mgr.SSHTunnelManager.SSHTunnelMessChan), 5},
		{"ShellMessChan", cap(mgr.ShellManager.ShellMessChan), 5},
		{"ListenMessChan", cap(mgr.ListenManager.ListenMessChan), 5},
		{"ConnectMessChan", cap(mgr.ConnectManager.ConnectMessChan), 5},
		{"OfflineMessChan", cap(mgr.OfflineManager.OfflineMessChan), 5},
		{"ChildrenTaskChan", cap(mgr.ChildrenManager.TaskChan), 0},
		{"SocksTaskChan", cap(mgr.SocksManager.TaskChan), 0},
		{"ForwardTaskChan", cap(mgr.ForwardManager.TaskChan), 0},
		{"BackwardTaskChan", cap(mgr.BackwardManager.TaskChan), 0},
	}

	for _, tc := range tests {
		if tc.cap != tc.want {
			t.Fatalf("%s cap = %d, want %d", tc.name, tc.cap, tc.want)
		}
	}
}
