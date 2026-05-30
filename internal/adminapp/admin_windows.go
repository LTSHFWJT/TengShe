//go:build windows

package adminapp

import (
	"TengShe/admin/cli"
	"TengShe/admin/initial"
	"TengShe/admin/printer"
	"TengShe/internal/adminbootstrap"
)

func Run() {
	printer.InitPrinter()

	options := initial.ParseOptions()

	cli.Banner()

	session := adminbootstrap.Connect(options)

	run(options, session)
}
