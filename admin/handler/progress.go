package handler

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const progressWidth = 32

type progressBar struct {
	total     int64
	current   int64
	started   bool
	finished  bool
	lastDraw  time.Time
	startTime time.Time
	mu        sync.Mutex
}

func newBar(length int64) *progressBar {
	if length < 0 {
		length = 0
	}
	return &progressBar{total: length}
}

// NewBar keeps the previous progress bar helper available to package callers.
func NewBar(length int64) *progressBar {
	return newBar(length)
}

func (bar *progressBar) Start() {
	bar.mu.Lock()
	defer bar.mu.Unlock()
	if bar.started {
		return
	}
	bar.started = true
	bar.startTime = time.Now()
	bar.drawLocked(true)
}

func (bar *progressBar) Add(scale int64) {
	bar.mu.Lock()
	defer bar.mu.Unlock()
	if bar.finished {
		return
	}
	if !bar.started {
		bar.started = true
		bar.startTime = time.Now()
	}
	bar.current += scale
	if bar.total > 0 && bar.current > bar.total {
		bar.current = bar.total
	}
	bar.drawLocked(false)
}

func (bar *progressBar) Add64(scale int64) {
	bar.Add(scale)
}

func (bar *progressBar) Finish() {
	bar.mu.Lock()
	defer bar.mu.Unlock()
	if bar.finished {
		return
	}
	bar.finished = true
	if bar.total > 0 {
		bar.current = bar.total
	}
	bar.drawLocked(true)
	fmt.Fprintln(os.Stderr)
}

func (bar *progressBar) drawLocked(force bool) {
	now := time.Now()
	if !force && now.Sub(bar.lastDraw) < 100*time.Millisecond {
		return
	}
	bar.lastDraw = now

	percent := float64(0)
	if bar.total > 0 {
		percent = float64(bar.current) / float64(bar.total)
	} else if bar.finished {
		percent = 1
	}
	if percent > 1 {
		percent = 1
	}

	filled := int(percent * progressWidth)
	if filled > progressWidth {
		filled = progressWidth
	}

	fmt.Fprintf(os.Stderr, "\r[%s%s] %6.2f%% %s/%s",
		repeatByte('=', filled),
		repeatByte(' ', progressWidth-filled),
		percent*100,
		formatBytes(bar.current),
		formatBytes(bar.total),
	)
}

func repeatByte(ch byte, n int) string {
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = ch
	}
	return string(buf)
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f%s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1fPB", value/unit)
}
