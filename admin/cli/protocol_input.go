package cli

import (
	"strings"

	"TengShe/share/transport/stream"
)

func parseProtocolChoice(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1", stream.ProtocolTCP:
		return stream.ProtocolTCP, true
	case "2", stream.ProtocolICMP:
		return stream.ProtocolICMP, true
	default:
		return "", false
	}
}

func parseConnectProtocol(args []string) (string, bool) {
	switch len(args) {
	case 2:
		return stream.ProtocolTCP, true
	case 3:
		return parseProtocolChoice(args[2])
	case 4:
		if args[2] != "-p" {
			return "", false
		}
		return parseProtocolChoice(args[3])
	default:
		return "", false
	}
}
