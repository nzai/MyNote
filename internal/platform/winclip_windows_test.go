package platform

import (
	"errors"
	"testing"

	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
)

func TestWinClipboardFormatMapping(t *testing.T) {
	for _, test := range []struct {
		name   string
		format core.Format
		want   co.CF
	}{
		{name: "text", format: core.FormatText, want: co.CF_UNICODETEXT},
		{name: "dibv5", format: core.FormatDIBV5, want: co.CF_DIBV5},
		{name: "dib", format: core.FormatDIB, want: co.CF_DIB},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := winClipboardFormat(test.format)
			if !ok || got != test.want {
				t.Fatalf("got %v, %v; want %v, true", got, ok, test.want)
			}
		})
	}

	if _, ok := winClipboardFormat(core.Format(99)); ok {
		t.Fatal("unknown format should not map")
	}
}

func TestClearOpenedClipboardEmptiesAndAlwaysCloses(t *testing.T) {
	emptyErr := errors.New("empty failed")
	closeErr := errors.New("close failed")
	session := &fakeClipboardClearSession{
		emptyErr: emptyErr,
		closeErr: closeErr,
	}

	err := clearOpenedClipboard(session)
	if !errors.Is(err, emptyErr) || !errors.Is(err, closeErr) {
		t.Fatalf("clear error = %v, want both empty and close errors", err)
	}
	if session.emptyCalls != 1 {
		t.Fatalf("EmptyClipboard calls = %d, want 1", session.emptyCalls)
	}
	if session.closeCalls != 1 {
		t.Fatalf("CloseClipboard calls = %d, want 1", session.closeCalls)
	}
}

type fakeClipboardClearSession struct {
	emptyErr   error
	closeErr   error
	emptyCalls int
	closeCalls int
}

func (s *fakeClipboardClearSession) EmptyClipboard() error {
	s.emptyCalls++
	return s.emptyErr
}

func (s *fakeClipboardClearSession) CloseClipboard() error {
	s.closeCalls++
	return s.closeErr
}
