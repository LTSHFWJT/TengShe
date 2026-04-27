package topology

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"TengShe/admin/printer"
)

func newTestTopology() *Topology {
	printer.Fail = func(format string, a ...interface{}) {}
	topology := NewTopology()
	topology.ResultChan = make(chan *topoResult, 20)
	return topology
}

func readTopoResult(t *testing.T, topology *Topology) *topoResult {
	t.Helper()

	select {
	case result := <-topology.ResultChan:
		return result
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for topology result")
	}

	return nil
}

func addTestNode(t *testing.T, topology *Topology, uuid string, parentUUID string, isFirst bool) int {
	t.Helper()

	topology.addNode(&TopoTask{
		ParentUUID: parentUUID,
		Target:     NewNode(uuid, "127.0.0.1"),
		IsFirst:    isFirst,
	})
	return readTopoResult(t, topology).IDNum
}

func TestTopologyAddCalculateAndGetRoute(t *testing.T) {
	topology := newTestTopology()

	if id := addTestNode(t, topology, "node1", "", true); id != 0 {
		t.Fatalf("node1 id = %d, want 0", id)
	}
	if id := addTestNode(t, topology, "node2", "node1", false); id != 1 {
		t.Fatalf("node2 id = %d, want 1", id)
	}
	if id := addTestNode(t, topology, "node3", "node2", false); id != 2 {
		t.Fatalf("node3 id = %d, want 2", id)
	}

	topology.calculate()
	readTopoResult(t, topology)

	tests := []struct {
		uuid      string
		wantRoute string
		wantHop   string
		wantHops  []string
	}{
		{uuid: "node1", wantRoute: "", wantHop: "", wantHops: nil},
		{uuid: "node2", wantRoute: "node2", wantHop: "node2", wantHops: []string{"node2"}},
		{uuid: "node3", wantRoute: "node2:node3", wantHop: "node2", wantHops: []string{"node2", "node3"}},
	}

	for _, tc := range tests {
		topology.getRoute(&TopoTask{UUID: tc.uuid})
		result := readTopoResult(t, topology)
		if got := result.Route; got != tc.wantRoute {
			t.Fatalf("route for %s = %q, want %q", tc.uuid, got, tc.wantRoute)
		}
		if result.RouteInfo.Wire != tc.wantRoute {
			t.Fatalf("route info wire for %s = %q, want %q", tc.uuid, result.RouteInfo.Wire, tc.wantRoute)
		}

		entry := topology.route[tc.uuid]
		if entry.Target != tc.uuid {
			t.Fatalf("route target for %s = %q, want %q", tc.uuid, entry.Target, tc.uuid)
		}
		if entry.NextHop != tc.wantHop {
			t.Fatalf("next hop for %s = %q, want %q", tc.uuid, entry.NextHop, tc.wantHop)
		}
		if !reflect.DeepEqual(entry.Hops, tc.wantHops) {
			t.Fatalf("hops for %s = %v, want %v", tc.uuid, entry.Hops, tc.wantHops)
		}
		if entry.Wire != tc.wantRoute {
			t.Fatalf("wire route for %s = %q, want %q", tc.uuid, entry.Wire, tc.wantRoute)
		}
	}
}

func TestTopologyGetRouteInfo(t *testing.T) {
	topology := newTestTopology()

	addTestNode(t, topology, "node1", "", true)
	addTestNode(t, topology, "node2", "node1", false)
	addTestNode(t, topology, "node3", "node2", false)

	topology.calculate()
	readTopoResult(t, topology)

	topology.getRouteInfo(&TopoTask{UUID: "node3"})
	result := readTopoResult(t, topology)

	if result.Route != "node2:node3" {
		t.Fatalf("route = %q, want node2:node3", result.Route)
	}
	if result.RouteInfo.Target != "node3" {
		t.Fatalf("target = %q, want node3", result.RouteInfo.Target)
	}
	if result.RouteInfo.NextHop != "node2" {
		t.Fatalf("next hop = %q, want node2", result.RouteInfo.NextHop)
	}
	if !reflect.DeepEqual(result.RouteInfo.Hops, []string{"node2", "node3"}) {
		t.Fatalf("hops = %v, want [node2 node3]", result.RouteInfo.Hops)
	}

	result.RouteInfo.Hops[0] = "changed"
	if got := topology.route["node3"].Hops[0]; got != "node2" {
		t.Fatalf("route info exposed mutable hops, got cached hop %q", got)
	}
}

func TestTopologyQueryMethodsUseRequestLane(t *testing.T) {
	topology := NewTopology()
	topology.ResultChan = make(chan *topoResult, 20)
	go topology.Run()

	_, id := topology.AddNode(NewNode("node1", "127.0.0.1"), "", true)
	if id != 0 {
		t.Fatalf("node1 id = %d, want 0", id)
	}
	_, id = topology.AddNode(NewNode("node2", "127.0.0.2"), "node1", false)
	if id != 1 {
		t.Fatalf("node2 id = %d, want 1", id)
	}
	_, id = topology.AddNode(NewNode("node3", "127.0.0.3"), "node2", false)
	if id != 2 {
		t.Fatalf("node3 id = %d, want 2", id)
	}

	topology.Recalculate()

	var wg sync.WaitGroup
	routes := make(chan RouteInfo, 2)
	for _, uuid := range []string{"node2", "node3"} {
		wg.Add(1)
		go func(uuid string) {
			defer wg.Done()
			routes <- topology.QueryRouteInfo(uuid)
		}(uuid)
	}
	wg.Wait()
	close(routes)

	got := map[string]RouteInfo{}
	for route := range routes {
		got[route.Target] = route
	}

	if got["node2"].Wire != "node2" {
		t.Fatalf("node2 wire route = %q, want node2", got["node2"].Wire)
	}
	if got["node3"].NextHop != "node2" || got["node3"].Wire != "node2:node3" {
		t.Fatalf("node3 route = %+v, want next hop node2 and wire node2:node3", got["node3"])
	}
	allRoutes := topology.QueryAllRouteInfo()
	if len(allRoutes) != 3 {
		t.Fatalf("all routes len = %d, want 3", len(allRoutes))
	}
	if len(topology.ResultChan) != 0 {
		t.Fatalf("legacy result channel has %d stale responses", len(topology.ResultChan))
	}
}

func TestTopologyDeleteAndReonline(t *testing.T) {
	topology := newTestTopology()

	addTestNode(t, topology, "node1", "", true)
	addTestNode(t, topology, "node2", "node1", false)
	addTestNode(t, topology, "node3", "node2", false)

	topology.calculate()
	readTopoResult(t, topology)

	topology.delNode(&TopoTask{UUID: "node2"})
	deleteResult := readTopoResult(t, topology)

	if want := []string{"node3", "node2"}; !reflect.DeepEqual(deleteResult.AllNodes, want) {
		t.Fatalf("deleted nodes = %v, want %v", deleteResult.AllNodes, want)
	}
	if _, ok := topology.nodes[1]; ok {
		t.Fatal("node2 still exists after delete")
	}
	if _, ok := topology.nodes[2]; ok {
		t.Fatal("node3 still exists after parent delete")
	}
	if children := topology.nodes[0].childrenUUID; len(children) != 0 {
		t.Fatalf("node1 children = %v, want empty", children)
	}
	if _, ok := topology.route["node2"]; ok {
		t.Fatal("node2 route still exists after delete")
	}
	if _, ok := topology.route["node3"]; ok {
		t.Fatal("node3 route still exists after delete")
	}

	topology.reonlineNode(&TopoTask{
		ParentUUID: "node1",
		Target:     NewNode("node2", "127.0.0.2"),
	})
	readTopoResult(t, topology)

	if got := topology.nodes[1]; got == nil || got.uuid != "node2" || got.parentUUID != "node1" {
		t.Fatalf("reonline node = %+v, want node2 under node1", got)
	}
	if topology.currentIDNum != 3 {
		t.Fatalf("current id = %d, want 3", topology.currentIDNum)
	}
	if children := topology.nodes[0].childrenUUID; !reflect.DeepEqual(children, []string{"node2"}) {
		t.Fatalf("node1 children = %v, want [node2]", children)
	}

	topology.calculate()
	readTopoResult(t, topology)
	topology.getRoute(&TopoTask{UUID: "node2"})
	if got := readTopoResult(t, topology).Route; got != "node2" {
		t.Fatalf("reonline route = %q, want node2", got)
	}

	if id := addTestNode(t, topology, "node4", "node1", false); id != 3 {
		t.Fatalf("node4 id = %d, want 3", id)
	}
}

func TestTopologyConsistencyChecks(t *testing.T) {
	topology := newTestTopology()

	if id := addTestNode(t, topology, "node1", "", true); id != 0 {
		t.Fatalf("node1 id = %d, want 0", id)
	}

	topology.addNode(&TopoTask{
		Target:  NewNode("node1", "127.0.0.2"),
		IsFirst: true,
	})
	duplicateResult := readTopoResult(t, topology)
	if !duplicateResult.IsExist || duplicateResult.IDNum != 0 {
		t.Fatalf("duplicate result = %+v, want existing id 0", duplicateResult)
	}
	if topology.currentIDNum != 1 || len(topology.nodes) != 1 {
		t.Fatalf("after duplicate currentID=%d nodes=%d, want currentID=1 nodes=1", topology.currentIDNum, len(topology.nodes))
	}

	topology.addNode(&TopoTask{
		ParentUUID: "missing-parent",
		Target:     NewNode("node2", "127.0.0.3"),
	})
	missingParentResult := readTopoResult(t, topology)
	if missingParentResult.IsExist || missingParentResult.IDNum != -1 {
		t.Fatalf("missing parent result = %+v, want failed id -1", missingParentResult)
	}
	if topology.currentIDNum != 1 || len(topology.nodes) != 1 {
		t.Fatalf("after missing parent currentID=%d nodes=%d, want currentID=1 nodes=1", topology.currentIDNum, len(topology.nodes))
	}
}

func TestTopologyLookupKeepsNodeNumbering(t *testing.T) {
	topology := newTestTopology()

	addTestNode(t, topology, "node1", "", true)
	addTestNode(t, topology, "node2", "node1", false)

	topology.getUUID(&TopoTask{UUIDNum: 0})
	if got := readTopoResult(t, topology).UUID; got != "node1" {
		t.Fatalf("uuid for id 0 = %q, want node1", got)
	}

	topology.getUUIDNum(&TopoTask{UUID: "node2"})
	if got := readTopoResult(t, topology).IDNum; got != 1 {
		t.Fatalf("id for node2 = %d, want 1", got)
	}

	topology.checkNode(&TopoTask{UUIDNum: 1})
	if got := readTopoResult(t, topology).IsExist; !got {
		t.Fatal("node id 1 should exist")
	}

	topology.checkNode(&TopoTask{UUIDNum: 2})
	if got := readTopoResult(t, topology).IsExist; got {
		t.Fatal("node id 2 should not exist")
	}
}

func TestTopologyDeleteDirectChildAndMissingNode(t *testing.T) {
	topology := newTestTopology()

	addTestNode(t, topology, "node1", "", true)
	addTestNode(t, topology, "node2", "node1", false)
	topology.calculate()
	readTopoResult(t, topology)

	topology.delNode(&TopoTask{UUID: "node1"})
	deleteResult := readTopoResult(t, topology)
	if want := []string{"node2", "node1"}; !reflect.DeepEqual(deleteResult.AllNodes, want) {
		t.Fatalf("deleted direct child subtree = %v, want %v", deleteResult.AllNodes, want)
	}
	if len(topology.nodes) != 0 {
		t.Fatalf("nodes after deleting direct child = %d, want 0", len(topology.nodes))
	}
	if len(topology.route) != 0 {
		t.Fatalf("routes after deleting direct child = %d, want 0", len(topology.route))
	}

	topology.delNode(&TopoTask{UUID: "missing-node"})
	missingResult := readTopoResult(t, topology)
	if len(missingResult.AllNodes) != 0 {
		t.Fatalf("missing node delete result = %v, want empty", missingResult.AllNodes)
	}
}
