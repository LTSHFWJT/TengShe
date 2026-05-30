//go:build !windows

package adminapp

import (
	"os"

	"TengShe/admin/cli"
	"TengShe/admin/initial"
	"TengShe/admin/printer"
	"TengShe/internal/adminbootstrap"

	"github.com/nsf/termbox-go"
)

func Run() {
	printer.InitPrinter()

	options := initial.ParseOptions()

	termbox.Init()
	termbox.SetCursor(0, 0)
	termbox.Flush()

	go listenCtrlC()

	cli.Banner()

	session := adminbootstrap.Connect(options)

	// Stop the temporary Ctrl-C watcher used while waiting for the first node.
	termbox.Interrupt()

	run(options, session)
}

func listenCtrlC() {
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
