package app

import (
	"TengShe/agent/initial"
	"TengShe/agent/process"
	"TengShe/internal/bootstrap"
	tsruntime "TengShe/internal/runtime"
)

func RunAgent() {
	options := initial.ParseOptions()

	agent := process.NewAgent(options)

	session := bootstrap.ConnectAgent(options)
	defer session.Cleanup()
	agent.UUID = session.UUID

	runtimeCtx := tsruntime.NewContext(session.Conn, options.Secret, agent.UUID, options.TlsEnable)
	runtimeCtx.ApplyGlobal()

	agent.Run()
}
