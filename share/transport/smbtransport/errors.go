package smbtransport

import "fmt"

type timeoutError struct {
	op   string
	addr string
}

func (err timeoutError) Error() string {
	if err.addr == "" {
		return err.op + " timeout"
	}
	return fmt.Sprintf("%s timeout on %s", err.op, err.addr)
}

func (timeoutError) Timeout() bool {
	return true
}

func (timeoutError) Temporary() bool {
	return true
}
