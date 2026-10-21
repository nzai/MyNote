//go:build windows

package ui

import (
	"os"
	"testing"

	"github.com/nzai/MyNote/internal/core"
)

func TestCreateTextWindow(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 smoke test")
	}
	window, err := createDocumentForTest(core.Snapshot{
		CaptureID: "test",
		Kind:      core.KindText,
		Text:      "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	window.Close()
}
