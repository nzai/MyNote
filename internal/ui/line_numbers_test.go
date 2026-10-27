//go:build windows

package ui

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func TestLogicalLineStartsUseEditorUTF16Offsets(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []int
	}{
		{
			name: "mixed line endings and surrogate pair",
			text: "A😀\r\nB\nC\rD",
			want: []int{0, 5, 8, 11},
		},
		{
			name: "trailing empty line",
			text: "first\n",
			want: []int{0, 7},
		},
		{
			name: "empty text",
			text: "",
			want: []int{0},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := logicalLineStarts(test.text); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("logicalLineStarts(%q) = %v, want %v", test.text, got, test.want)
			}
		})
	}
}

func TestLineNumberGutterWidthGrowsWithDigits(t *testing.T) {
	oneDigit := lineNumberGutterWidthDIP(9)
	fourDigits := lineNumberGutterWidthDIP(1000)
	sevenDigits := lineNumberGutterWidthDIP(1000000)
	if fourDigits <= oneDigit {
		t.Fatalf("four-digit gutter = %d, want greater than one-digit %d", fourDigits, oneDigit)
	}
	if sevenDigits <= fourDigits {
		t.Fatalf("seven-digit gutter = %d, want greater than four-digit %d", sevenDigits, fourDigits)
	}
}

func TestEditorViewportMessagesTriggerLineNumberSync(t *testing.T) {
	for _, message := range []co.WM{
		co.EM_LINESCROLL,
		co.WM_VSCROLL,
		co.WM_KEYDOWN,
		wmMouseWheel,
	} {
		if !editorMessageMayMoveViewport(message) {
			t.Fatalf("message %#x must trigger line-number synchronization", uint32(message))
		}
	}
	if editorMessageMayMoveViewport(co.EM_GETFIRSTVISIBLELINE) {
		t.Fatal("line-number viewport query must not recursively trigger synchronization")
	}
}

func TestTextDocumentUsesPreviewTitleAndLogicalLineNumbers(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 line-number test")
	}
	lockGUIThread(t)
	document, err := NewDocument(DocumentOptions{
		Snapshot: core.Snapshot{
			CaptureID: "line-number-test",
			Kind:      core.KindText,
			Text:      "第一段预览标题\n第二行",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = document.native.hwnd.ShowWindow(co.SW_HIDE)
	defer func() {
		_ = document.native.hwnd.DestroyWindow()
	}()

	title, err := document.native.hwnd.GetWindowText()
	if err != nil {
		t.Fatal(err)
	}
	if title != "第一段预览标题 第二行" {
		t.Fatalf("window title = %q, want content preview", title)
	}

	setEditorText(document, strings.Repeat("wrapped ", 80)+"\r\nsecond")
	rows, err := document.visibleLineNumberRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].number != 1 || rows[1].number != 2 {
		t.Fatalf("visible line-number rows = %+v, want logical lines 1 and 2", rows)
	}
	lineHeight := document.fontLineHeight()
	if rows[1].y-rows[0].y <= lineHeight {
		t.Fatalf(
			"wrapped first line did not move line 2 down: row1=%d row2=%d lineHeight=%d",
			rows[0].y,
			rows[1].y,
			lineHeight,
		)
	}

	setEditorText(document, "first\r\nsecond\r\n")
	rows, err = document.visibleLineNumberRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[2].number != 3 {
		t.Fatalf("trailing empty line rows = %+v, want line 3", rows)
	}

	setEditorText(document, strings.Repeat("line\r\n", 200))
	_, _ = document.edit.SendMessage(co.EM_LINESCROLL, 0, win.LPARAM(100))
	rows, err = document.visibleLineNumberRows()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || rows[0].number == 1 {
		t.Fatalf("scrolled line-number rows = %+v, want a later logical line first", rows)
	}
}

func TestVisibleLineNumbersKeepBlankLinesAfterWrappedText(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 line-number test")
	}
	lockGUIThread(t)
	text := "**已完成**\n\n" +
		"- 文本编辑框自动折行，不再出现横向滚动条；内容超高时才显示纵向滚动条。\n" +
		"- 左侧增加行号，并与原始文本行对齐；自动折行的续行留空，不重复编号。\n" +
		"- 滚动编辑框时，行号同步移动。\n" +
		"- 窗口标题和任务栏标题改为内容预览：文本取前 32 个字符，超长加省略号；图片显示 `（图片） 1920 × 1080`；空文本显示 `（空文本）`。\n" +
		"- 标题预览只在内容变化时计算，不会在每次重绘时扫描整段文本。"
	document, err := NewDocument(DocumentOptions{
		Snapshot: core.Snapshot{
			CaptureID: "blank-line-number-test",
			Kind:      core.KindText,
			Text:      text,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = document.native.hwnd.ShowWindow(co.SW_HIDE)
	defer func() {
		_ = document.native.hwnd.DestroyWindow()
	}()

	rows, err := document.visibleLineNumberRows()
	if err != nil {
		t.Fatal(err)
	}
	got := make([]int, len(rows))
	for index, row := range rows {
		got[index] = row.number
	}
	want := []int{1, 2, 3, 4, 5, 6, 7}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("visible line numbers = %v, want %v (rows=%+v)", got, want, rows)
	}
	for index := 1; index < len(rows); index++ {
		if rows[index].y <= rows[index-1].y {
			t.Fatalf(
				"logical line %d starts at y=%d, not after line %d at y=%d: %+v",
				rows[index].number,
				rows[index].y,
				rows[index-1].number,
				rows[index-1].y,
				rows,
			)
		}
	}
}

func TestLineNumberGutterCoversLeftContentPadding(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 line-number test")
	}
	lockGUIThread(t)
	document, err := NewDocument(DocumentOptions{
		Snapshot: core.Snapshot{
			CaptureID: "line-number-left-edge-test",
			Kind:      core.KindText,
			Text:      "line number gutter",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = document.native.hwnd.ShowWindow(co.SW_HIDE)
	defer func() {
		_ = document.native.hwnd.DestroyWindow()
	}()

	screen, err := win.HWND(0).GetDC()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = win.HWND(0).ReleaseDC(screen)
	}()
	target, err := screen.CreateCompatibleDC()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = target.DeleteDC()
	}()
	bitmap, err := screen.CreateCompatibleBitmap(640, 400)
	if err != nil {
		t.Fatal(err)
	}
	oldBitmap, err := target.SelectObjectBmp(bitmap)
	if err != nil {
		_ = bitmap.DeleteObject()
		t.Fatal(err)
	}
	defer func() {
		_, _ = target.SelectObjectBmp(oldBitmap)
		_ = bitmap.DeleteObject()
	}()
	background, err := brushFor(colorWindowBackground)
	if err != nil {
		t.Fatal(err)
	}
	full := win.RECT{Right: 640, Bottom: 400}
	if err := target.FillRect(&full, background); err != nil {
		t.Fatal(err)
	}

	dpi := dpiFor(document.native.hwnd)
	titleHeight := scaleDIP(titlebarHeightDIP, dpi)
	statusHeight := scaleDIP(statusbarHeightDIP, dpi)
	if err := document.paintLineNumberGutter(
		target,
		640,
		400,
		titleHeight,
		statusHeight,
		dpi,
	); err != nil {
		t.Fatal(err)
	}
	pixel, err := pixelColor(target, win.POINT{
		X: 0,
		Y: int32(titleHeight + scaleDIP(20, dpi)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if pixel != colorLineGutter {
		t.Fatalf(
			"line-number gutter left pixel = %#08x, want gutter color %#08x",
			pixel,
			colorLineGutter,
		)
	}
}

func TestTextDocumentLoadPreservesOriginalTextAndCleanState(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 document load test")
	}
	lockGUIThread(t)
	text := "first\nsecond\n"
	document, err := NewDocument(DocumentOptions{
		Snapshot: core.Snapshot{
			CaptureID: "document-load-test",
			Kind:      core.KindText,
			Text:      text,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = document.native.hwnd.ShowWindow(co.SW_HIDE)
	defer func() {
		_ = document.native.hwnd.DestroyWindow()
	}()

	if document.saveState.Text() != text {
		t.Fatalf("loaded text = %q, want original %q", document.saveState.Text(), text)
	}
	if document.saveState.Dirty() {
		t.Fatal("loading clipboard text must not mark the document dirty")
	}
}
