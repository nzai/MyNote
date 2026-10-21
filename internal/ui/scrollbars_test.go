//go:build windows

package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func TestScrollRangeExceedsPage(t *testing.T) {
	tests := []struct {
		name string
		info win.SCROLLINFO
		want bool
	}{
		{name: "empty", info: win.SCROLLINFO{Min: 0, Max: 0, Page: 1}},
		{name: "exact page", info: win.SCROLLINFO{Min: 0, Max: 33, Page: 34}},
		{name: "one more unit", info: win.SCROLLINFO{Min: 0, Max: 34, Page: 34}, want: true},
		{name: "nonzero minimum", info: win.SCROLLINFO{Min: 10, Max: 19, Page: 10}},
		{name: "nonzero minimum overflow", info: win.SCROLLINFO{Min: 10, Max: 20, Page: 10}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := scrollRangeExceedsPage(test.info); got != test.want {
				t.Fatalf("scrollRangeExceedsPage() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestTextEditorShowsOnlyNeededScrollbars(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 scrollbar test")
	}
	document, err := NewDocument(DocumentOptions{
		Snapshot: core.Snapshot{
			CaptureID: "scrollbar-test",
			Kind:      core.KindText,
			Text:      "short",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = document.native.hwnd.ShowWindow(co.SW_HIDE)
	defer func() {
		_ = document.native.hwnd.DestroyWindow()
	}()

	assertScrollbars(t, document, false, false)

	setEditorText(document, strings.Repeat("x", 5000))
	assertScrollbars(t, document, false, true)

	setEditorText(document, strings.Repeat("line\r\n", 200))
	assertScrollbars(t, document, false, true)
}

func setEditorText(document *Document, text string) {
	_ = document.edit.SetWindowText(text)
	document.handleCommand(win.WPARAM(
		uint32(editControlID) | uint32(uint16(co.EN_CHANGE))<<16,
	))
}

func assertScrollbars(t *testing.T, document *Document, wantHorizontal, wantVertical bool) {
	t.Helper()
	outer, err := document.edit.GetWindowRect()
	if err != nil {
		t.Fatal(err)
	}
	inner, err := document.edit.GetClientRect()
	if err != nil {
		t.Fatal(err)
	}
	const scrollbarThreshold = 5
	gotHorizontal := int(outer.Bottom-outer.Top-inner.Bottom) >= scrollbarThreshold
	gotVertical := int(outer.Right-outer.Left-inner.Right) >= scrollbarThreshold
	if gotHorizontal != wantHorizontal || gotVertical != wantVertical {
		var horizontal win.SCROLLINFO
		horizontal.Mask = co.SIF_RANGE | co.SIF_PAGE
		horizontal.SetCbSize()
		_ = document.edit.GetScrollInfo(co.SBB_HORZ, &horizontal)
		var vertical win.SCROLLINFO
		vertical.Mask = co.SIF_RANGE | co.SIF_PAGE
		vertical.SetCbSize()
		_ = document.edit.GetScrollInfo(co.SBB_VERT, &vertical)
		t.Fatalf(
			"scrollbars horizontal=%v vertical=%v, want horizontal=%v vertical=%v (tracked=%+v, h-range=%d..%d page=%d, v-range=%d..%d page=%d, outer %dx%d, client %dx%d)",
			gotHorizontal,
			gotVertical,
			wantHorizontal,
			wantVertical,
			document.scrollbars,
			horizontal.Min,
			horizontal.Max,
			horizontal.Page,
			vertical.Min,
			vertical.Max,
			vertical.Page,
			outer.Right-outer.Left,
			outer.Bottom-outer.Top,
			inner.Right,
			inner.Bottom,
		)
	}
}
