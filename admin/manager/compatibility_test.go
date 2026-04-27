package manager

import (
	"testing"

	"TengShe/share"
)

func TestAdminManagerTaskModeValuesRemainCompatible(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"S_NEWSOCKS", S_NEWSOCKS, 0},
		{"S_ADDTCPSOCKET", S_ADDTCPSOCKET, 1},
		{"S_GETNEWSEQ", S_GETNEWSEQ, 2},
		{"S_GETTCPDATACHAN", S_GETTCPDATACHAN, 3},
		{"S_GETUDPDATACHAN", S_GETUDPDATACHAN, 4},
		{"S_GETTCPDATACHAN_WITHOUTUUID", S_GETTCPDATACHAN_WITHOUTUUID, 5},
		{"S_GETUDPDATACHAN_WITHOUTUUID", S_GETUDPDATACHAN_WITHOUTUUID, 6},
		{"S_CLOSETCP", S_CLOSETCP, 7},
		{"S_GETUDPSTARTINFO", S_GETUDPSTARTINFO, 8},
		{"S_UPDATEUDP", S_UPDATEUDP, 9},
		{"S_GETSOCKSINFO", S_GETSOCKSINFO, 10},
		{"S_CLOSESOCKS", S_CLOSESOCKS, 11},
		{"S_FORCESHUTDOWN", S_FORCESHUTDOWN, 12},
		{"F_GETNEWSEQ", F_GETNEWSEQ, 0},
		{"F_NEWFORWARD", F_NEWFORWARD, 1},
		{"F_ADDCONN", F_ADDCONN, 2},
		{"F_GETDATACHAN", F_GETDATACHAN, 3},
		{"F_GETDATACHAN_WITHOUTUUID", F_GETDATACHAN_WITHOUTUUID, 4},
		{"F_GETFORWARDINFO", F_GETFORWARDINFO, 5},
		{"F_CLOSETCP", F_CLOSETCP, 6},
		{"F_CLOSESINGLE", F_CLOSESINGLE, 7},
		{"F_CLOSESINGLEALL", F_CLOSESINGLEALL, 8},
		{"F_FORCESHUTDOWN", F_FORCESHUTDOWN, 9},
		{"B_NEWBACKWARD", B_NEWBACKWARD, 0},
		{"B_GETNEWSEQ", B_GETNEWSEQ, 1},
		{"B_ADDCONN", B_ADDCONN, 2},
		{"B_CHECKBACKWARD", B_CHECKBACKWARD, 3},
		{"B_GETDATACHAN", B_GETDATACHAN, 4},
		{"B_GETDATACHAN_WITHOUTUUID", B_GETDATACHAN_WITHOUTUUID, 5},
		{"B_CLOSETCP", B_CLOSETCP, 6},
		{"B_GETBACKWARDINFO", B_GETBACKWARDINFO, 7},
		{"B_GETSTOPRPORT", B_GETSTOPRPORT, 8},
		{"B_CLOSESINGLE", B_CLOSESINGLE, 9},
		{"B_CLOSESINGLEALL", B_CLOSESINGLEALL, 10},
		{"B_FORCESHUTDOWN", B_FORCESHUTDOWN, 11},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Fatalf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func TestAdminManagerChannelCompatibility(t *testing.T) {
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
		{"InfoMessChan", cap(mgr.InfoManager.InfoMessChan), 5},
		{"ListenMessChan", cap(mgr.ListenManager.ListenMessChan), 5},
		{"ConnectMessChan", cap(mgr.ConnectManager.ConnectMessChan), 5},
		{"ChildrenMessChan", cap(mgr.ChildrenManager.ChildrenMessChan), 5},
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
