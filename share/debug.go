package share

import (
	"log"
	"os"
	"strings"
)

const DebugEnv = "TENGSHE_DEBUG"

func DebugEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(DebugEnv)))
	return value != "" && value != "0" && value != "false" && value != "off"
}

func Debugf(format string, args ...interface{}) {
	if DebugEnabled() {
		log.Printf("[debug] "+format, args...)
	}
}
