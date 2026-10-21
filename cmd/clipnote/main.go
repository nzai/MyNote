package main

import (
	"os"
	"runtime"

	"github.com/nzai/MyNote/internal/ui"
)

func main() {
	runtime.LockOSThread()
	os.Exit(ui.Run(ui.Options{}))
}
