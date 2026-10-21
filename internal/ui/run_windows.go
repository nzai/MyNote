//go:build windows

package ui

import (
	"errors"
	"fmt"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

type Options struct{}

func Run(options Options) int {
	_ = options
	if err := win.SetProcessDpiAwarenessContext(
		co.DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2,
	); err != nil {
		// The process may already have inherited DPI awareness from a host.
		_ = win.SetProcessDPIAware()
	}
	if _, err := registerNativeClass(documentWindowClass); err != nil {
		showFatalUIError(err)
		return 1
	}

	exitCode, runErr := runMessageLoop()
	resourceErr := releaseUIResources()
	if err := errors.Join(runErr, resourceErr); err != nil {
		showFatalUIError(err)
		return 1
	}
	return exitCode
}

func showFatalUIError(err error) {
	if err == nil {
		return
	}
	_, _ = win.HWND(0).MessageBox(fmt.Sprintf("%v", err), "ClipNote 错误", co.MB_OK|co.MB_ICONERROR)
}
