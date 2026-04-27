package runtime

import (
	"net"
	"testing"

	"TengShe/global"
	"TengShe/protocol"
)

func TestContextApplyAndAccessors(t *testing.T) {
	oldCurrent := current
	oldComponent := global.G_Component
	oldTLSEnable := global.G_TLSEnable
	defer func() {
		current = oldCurrent
		global.G_Component = oldComponent
		global.G_TLSEnable = oldTLSEnable
	}()

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	ctx := NewContext(client, "secret", "NODE000001", true)
	defer ctx.Sender.Close()
	ctx.ApplyGlobal()

	if Current() != ctx {
		t.Fatal("Current did not return applied context")
	}
	if Component() != ctx.Component {
		t.Fatal("Component did not return context component")
	}
	if !TLSEnabled() {
		t.Fatal("TLSEnabled = false, want true")
	}
	if global.G_Component != ctx.Component || !global.G_TLSEnable {
		t.Fatal("ApplyGlobal did not preserve global compatibility")
	}
}

func TestUpdateConnPreservesGlobalCompatibility(t *testing.T) {
	oldCurrent := current
	oldComponent := global.G_Component
	oldTLSEnable := global.G_TLSEnable
	defer func() {
		current = oldCurrent
		global.G_Component = oldComponent
		global.G_TLSEnable = oldTLSEnable
	}()

	first, firstPeer := net.Pipe()
	defer first.Close()
	defer firstPeer.Close()

	second, secondPeer := net.Pipe()
	defer second.Close()
	defer secondPeer.Close()

	ctx := NewContext(first, "secret", "NODE000001", false)
	defer ctx.Sender.Close()
	ctx.ApplyGlobal()
	UpdateConn(second)
	defer ctx.Sender.Close()

	if ctx.Component.Conn != second {
		t.Fatal("context conn was not updated")
	}
	if global.G_Component.Conn != second {
		t.Fatal("global conn was not updated")
	}
}

func TestDownstreamConnSelectsDirectOrFirstHop(t *testing.T) {
	oldCurrent := current
	oldComponent := global.G_Component
	oldTLSEnable := global.G_TLSEnable
	defer func() {
		current = oldCurrent
		global.G_Component = oldComponent
		global.G_TLSEnable = oldTLSEnable
	}()

	defaultConn, defaultPeer := net.Pipe()
	defer defaultConn.Close()
	defer defaultPeer.Close()
	directConn, directPeer := net.Pipe()
	defer directPeer.Close()
	hopConn, hopPeer := net.Pipe()
	defer hopPeer.Close()

	ctx := NewContext(defaultConn, "secret", protocol.ADMIN_UUID, false)
	defer ctx.Sender.Close()
	ctx.ApplyGlobal()
	ctx.RegisterDownstreamConn("node-direct", directConn)
	ctx.RegisterDownstreamConn("node-hop", hopConn)
	defer ctx.UnregisterDownstreamConn("node-direct")
	defer ctx.UnregisterDownstreamConn("node-hop")

	if got := DownstreamConn("node-direct", ""); got != directConn {
		t.Fatal("direct downstream route did not use accepter connection")
	}
	if ctx.UnregisterDownstreamConnIfMatch("node-direct", defaultConn) {
		t.Fatal("stale downstream conn unexpectedly unregistered active route")
	}
	if got := DownstreamConn("node-direct", ""); got != directConn {
		t.Fatal("direct downstream route changed after stale unregister")
	}
	if got := DownstreamConn("node-target", "node-hop:node-target"); got != hopConn {
		t.Fatal("multihop downstream route did not use first hop connection")
	}
	if got := DownstreamConn("missing", ""); got != defaultConn {
		t.Fatal("missing downstream route did not fall back to default connection")
	}
}

func TestCurrentFallsBackToGlobal(t *testing.T) {
	oldCurrent := current
	oldComponent := global.G_Component
	oldTLSEnable := global.G_TLSEnable
	defer func() {
		current = oldCurrent
		global.G_Component = oldComponent
		global.G_TLSEnable = oldTLSEnable
	}()

	current = nil
	global.G_Component = &protocol.MessageComponent{Secret: "global-secret", UUID: "NODE000002"}
	global.G_TLSEnable = true

	if Component() != global.G_Component {
		t.Fatal("Component did not fall back to global component")
	}
	if !TLSEnabled() {
		t.Fatal("TLSEnabled did not fall back to global flag")
	}
}
