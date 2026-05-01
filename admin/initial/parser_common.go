package initial

import (
	"fmt"
	"strings"
)

func normalizeProtocolFlagArgs(args []string) []string {
	normalized := make([]string, len(args))
	copy(normalized, args)
	for i, arg := range normalized {
		switch arg {
		case "-transport", "--transport":
			normalized[i] = "-p"
		default:
			if value, ok := strings.CutPrefix(arg, "-transport="); ok {
				normalized[i] = "-p=" + value
				continue
			}
			if value, ok := strings.CutPrefix(arg, "--transport="); ok {
				normalized[i] = "-p=" + value
			}
		}
	}
	return normalized
}

func validateDownstream(value string) error {
	switch value {
	case "", "raw", "http":
		return nil
	case "ws", "websocket":
		return fmt.Errorf("ws message wrapping was removed; use -p ws for WebSocket transport")
	default:
		return fmt.Errorf("unsupported downstream data type %q: expected raw or http", value)
	}
}
