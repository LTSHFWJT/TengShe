//go:build !windows

package main

import (
	"runtime"

	"TengShe/internal/adminapp"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	adminapp.Run()
}
