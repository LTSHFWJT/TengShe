package adminapp

import (
	"TengShe/admin/initial"
	"TengShe/admin/process"
	"TengShe/internal/adminbootstrap"
	tsruntime "TengShe/internal/runtime"
	"TengShe/protocol"
)

func run(options *initial.Options, session *adminbootstrap.Session) {
	topo := session.Topology
	admin := process.NewAdmin(options, topo, session.Root, session.Accepted)

	topo.Recalculate()

	if session.Root == nil {
		return
	}
	runtimeCtx := tsruntime.NewContext(session.Root.Conn, options.Secret, protocol.ADMIN_UUID, false)
	runtimeCtx.RegisterDownstreamConn(session.Root.UUID, session.Root.Conn)
	runtimeCtx.ApplyGlobal()

	admin.Run()
}
