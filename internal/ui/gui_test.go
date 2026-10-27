//go:build windows

package ui

import (
	"runtime"
	"testing"
)

func lockGUIThread(t *testing.T) {
	t.Helper()
	// Native windows are thread-affine; without this, the Go scheduler may
	// migrate a test between creating a RichEdit control and messaging it.
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
}
