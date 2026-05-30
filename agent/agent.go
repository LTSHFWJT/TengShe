package main

import (
	"runtime"

	"TengShe/internal/agentapp"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	agentapp.Run()
}
