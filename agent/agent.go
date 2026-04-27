package main

import (
	"runtime"

	"TengShe/internal/app"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	app.RunAgent()
}
