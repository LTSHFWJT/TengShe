package initial

import "strings"

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
