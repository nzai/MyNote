package platform

import (
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
