package process

import (
	"TengShe/admin/manager"
	"TengShe/admin/printer"
	"TengShe/admin/topology"
	"TengShe/protocol"
)

func nodeOffline(mgr *manager.Manager, topo *topology.Topology, uuid string) {
	allNodes := topo.DeleteNode(uuid)

	for _, nodeUUID := range allNodes {
		backwardTask := &manager.BackwardTask{
			Mode: manager.B_FORCESHUTDOWN,
			UUID: nodeUUID,
		}
		mgr.BackwardManager.TaskChan <- backwardTask
		<-mgr.BackwardManager.ResultChan

		forwardTask := &manager.ForwardTask{
			Mode: manager.F_FORCESHUTDOWN,
			UUID: nodeUUID,
		}
		mgr.ForwardManager.TaskChan <- forwardTask
		<-mgr.ForwardManager.ResultChan

		socksTask := &manager.SocksTask{
			Mode: manager.S_FORCESHUTDOWN,
			UUID: nodeUUID,
		}
		mgr.SocksManager.TaskChan <- socksTask
		<-mgr.SocksManager.ResultChan
	}

	topo.Recalculate()
}

func nodeReonline(mgr *manager.Manager, topo *topology.Topology, mess *protocol.NodeReonline) {
	node := topology.NewNode(mess.UUID, mess.IP)

	topo.ReonlineNode(node, mess.ParentUUID, false)
	topo.Recalculate()

	printer.Success("\r\n[*] Node %d is back online!", topo.QueryUUIDNum(mess.UUID))
}

func DispatchChildrenMess(mgr *manager.Manager, topo *topology.Topology) {
	for {
		message := <-mgr.ChildrenManager.ChildrenMessChan

		switch mess := message.(type) {
		case *protocol.NodeOffline:
			nodeOffline(mgr, topo, mess.UUID)
		case *protocol.NodeReonline:
			nodeReonline(mgr, topo, mess)
		}
	}
}
