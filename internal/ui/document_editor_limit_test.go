//go:build windows

package ui

import (
	"strings"
	"testing"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func TestTextEditorRaisesTextLimit(t *testing.T) {
	document := newSelectionTestDocument(t, "hello")

	limit, err := document.edit.SendMessage(co.EM_GETLIMITTEXT, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if int(limit) <= 32767 {
		t.Fatalf(
			"editor text limit = %d; RichEdit defaults to 32767, which rejects typing in large documents",
			limit,
		)
	}
}

func TestTextEditorAcceptsTypingAfterLargeText(t *testing.T) {
	document := newSelectionTestDocument(t, strings.Repeat("a", 40000))

	before, err := document.edit.GetWindowTextLength()
	if err != nil {
		t.Fatal(err)
	}
	if before != 40000 {
		t.Fatalf("editor length = %d, want 40000", before)
	}
	if _, err := document.edit.SendMessage(co.WM_CHAR, win.WPARAM('X'), 0); err != nil {
		t.Fatal(err)
	}
	after, err := document.edit.GetWindowTextLength()
	if err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Fatalf(
			"editor length after typing = %d, want %d; the text limit still rejects input",
			after, before+1,
		)
	}
}
