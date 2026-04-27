//go:build !windows

package app

import (
	"os"

	"TengShe/admin/cli"
	"TengShe/admin/initial"
	"TengShe/admin/printer"
	"TengShe/internal/bootstrap"

	"github.com/nsf/termbox-go"
)

func RunAdmin() {
	printer.InitPrinter()

	options := initial.ParseOptions()

	termbox.Init()
	termbox.SetCursor(0, 0)
	termbox.Flush()

	go listenAdminCtrlC()

	cli.Banner()

	session := bootstrap.ConnectAdmin(options)

	// Stop the temporary Ctrl-C watcher used while waiting for the first node.
	termbox.Interrupt()

	runAdmin(options, session)
}

func listenAdminCtrlC() {
	for {
		event := termbox.PollEvent()
		if event.Type == termbox.EventInterrupt {
			break
		}

		if event.Key == termbox.KeyCtrlC {
			termbox.Close()
			os.Exit(0)
		}
	}
}
