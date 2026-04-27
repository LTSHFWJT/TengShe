//go:build windows

package app

import (
	"TengShe/admin/cli"
	"TengShe/admin/initial"
	"TengShe/admin/printer"
	"TengShe/internal/bootstrap"
)

func RunAdmin() {
	printer.InitPrinter()

	options := initial.ParseOptions()

	cli.Banner()

	session := bootstrap.ConnectAdmin(options)

	runAdmin(options, session)
}
