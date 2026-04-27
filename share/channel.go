package share

import "time"

const DefaultDataEnqueueTimeout = 30 * time.Second

func EnqueueBytes(ch chan []byte, data []byte, timeout time.Duration) (ok bool) {
	if ch == nil {
		return false
	}

	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	select {
	case ch <- data:
		return true
	default:
	}

	if timeout <= 0 {
		ch <- data
		return true
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case ch <- data:
		return true
	case <-timer.C:
		return false
	}
}

func CloseBytesChanQuietly(ch chan []byte) {
	defer func() { _ = recover() }()
	if ch != nil {
		close(ch)
	}
}

func CloseUint64ChanQuietly(ch chan uint64) {
	defer func() { _ = recover() }()
	if ch != nil {
		close(ch)
	}
}
