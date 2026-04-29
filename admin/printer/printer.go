package printer

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/fatih/color"
)

var (
	Warning func(format string, a ...interface{})
	Success func(format string, a ...interface{})
	Fail    func(format string, a ...interface{})
)

type PromptState struct {
	Visible bool
	Status  string
	Left    string
	Right   string
}

var (
	outputMu       sync.Mutex
	promptMu       sync.RWMutex
	promptProvider func() PromptState
)

func InitPrinter() {
	Warning = coloredPrintf(color.New(color.FgYellow))
	Success = coloredPrintf(color.New(color.FgGreen))
	Fail = coloredPrintf(color.New(color.FgRed))
}

func SetPromptProvider(provider func() PromptState) {
	promptMu.Lock()
	promptProvider = provider
	promptMu.Unlock()
}

func RedrawPrompt() {
	outputMu.Lock()
	defer outputMu.Unlock()
	state := currentPromptState()
	if state.Visible {
		redrawPrompt(state)
	}
}

func Print(format string, a ...interface{}) {
	printWithColor(nil, format, a...)
}

func coloredPrintf(c *color.Color) func(format string, a ...interface{}) {
	return func(format string, a ...interface{}) {
		printWithColor(c, format, a...)
	}
}

func printWithColor(c *color.Color, format string, a ...interface{}) {
	output := fmt.Sprintf(format, a...)
	outputMu.Lock()
	defer outputMu.Unlock()

	state := currentPromptState()
	if state.Visible {
		fmt.Fprint(os.Stdout, "\r\033[K")
	}

	if c == nil {
		fmt.Fprint(os.Stdout, output)
	} else {
		_, _ = c.Fprint(os.Stdout, output)
	}

	if state.Visible {
		if !strings.HasSuffix(output, "\n") && !strings.HasSuffix(output, "\r") {
			fmt.Fprint(os.Stdout, "\r\n")
		}
		redrawPrompt(state)
	}
}

func currentPromptState() PromptState {
	promptMu.RLock()
	provider := promptProvider
	promptMu.RUnlock()
	if provider == nil {
		return PromptState{}
	}
	return provider()
}

func redrawPrompt(state PromptState) {
	fmt.Fprint(os.Stdout, "\r\033[K")
	fmt.Fprint(os.Stdout, state.Status)
	fmt.Fprint(os.Stdout, state.Left+state.Right)
	moveCursorLeft(displayWidth(state.Right))
}

func moveCursorLeft(width int) {
	if width <= 0 {
		return
	}
	fmt.Fprintf(os.Stdout, "\033[%dD", width)
}

func displayWidth(value string) int {
	width := 0
	for _, r := range value {
		if r > 127 {
			width += 2
		} else {
			width++
		}
	}
	return width
}
