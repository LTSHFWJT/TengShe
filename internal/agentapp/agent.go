package agentapp

import (
	"TengShe/agent/initial"
	"TengShe/agent/process"
	"TengShe/internal/agentbootstrap"
	tsruntime "TengShe/internal/runtime"
)

func Run() {
	options := initial.ParseOptions()

	agent := process.NewAgent(options)

	session := agentbootstrap.Connect(options)
	defer session.Cleanup()
	agent.UUID = session.UUID

	runtimeCtx := tsruntime.NewContext(session.Conn, options.Secret, agent.UUID, options.TlsEnable)
	runtimeCtx.ApplyGlobal()

	agent.Run()
}
