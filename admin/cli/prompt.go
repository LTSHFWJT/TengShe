package cli

import "TengShe/admin/printer"

func (console *Console) installPromptProvider() {
	printer.SetPromptProvider(console.promptState)
}

func (console *Console) uninstallPromptProvider() {
	printer.SetPromptProvider(nil)
}

func (console *Console) setPromptLive(live bool) {
	console.renderMu.Lock()
	console.promptLive = live
	console.capturePromptStateLocked()
	console.renderMu.Unlock()
}

func (console *Console) setPromptInput(left, right string) {
	console.renderMu.Lock()
	console.leftInput = left
	console.rightInput = right
	console.capturePromptStateLocked()
	console.renderMu.Unlock()
}

func (console *Console) redrawPrompt() {
	printer.RedrawPrompt()
}

func (console *Console) promptState() printer.PromptState {
	console.renderMu.RLock()
	defer console.renderMu.RUnlock()
	return printer.PromptState{
		Visible: console.promptLive && !console.promptShell && !console.promptSSH,
		Status:  console.promptText,
		Left:    console.leftInput,
		Right:   console.rightInput,
	}
}

func (console *Console) capturePromptStateLocked() {
	console.promptText = console.status
	console.promptShell = console.shellMode
	console.promptSSH = console.sshMode
}
