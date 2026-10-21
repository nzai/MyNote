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

func TestTextDocumentUsesPreviewTitleAndLogicalLineNumbers(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 line-number test")
	}
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
