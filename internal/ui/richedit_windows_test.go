//go:build windows

package ui

import (
	"strconv"
	"testing"
	"unsafe"

	"github.com/nzai/MyNote/internal/config"
)

func TestRichEditCharacterFormatMapsEditorSettings(t *testing.T) {
	settings := config.EditorConfig{
		FontFamily:    "Cascadia Mono",
		FontPointSize: 18,
		LineSpacing:   125,
	}
	format := richEditCharacterFormat(settings)

	if format.CbSize != uint32(unsafe.Sizeof(format)) {
		t.Fatalf("cbSize = %d, want %d", format.CbSize, unsafe.Sizeof(format))
	}
	const wantMask = cfmFace | cfmSize | cfmColor | cfmWeight | cfmCharSet
	if format.DwMask != wantMask {
		t.Fatalf("mask = %#x, want %#x", format.DwMask, wantMask)
	}
	if format.YHeight != 18*20 {
		t.Fatalf("height = %d, want %d", format.YHeight, 18*20)
	}
	if format.WWeight != 400 {
		t.Fatalf("weight = %d, want 400", format.WWeight)
	}
	if got := faceNameFromRichEditFormat(&format); got != settings.FontFamily {
		t.Fatalf("face = %q, want %q", got, settings.FontFamily)
	}
}

func TestRichEditParagraphFormatUsesMultipleLineSpacing(t *testing.T) {
	tests := []struct {
		percent int
		value   int32
	}{
		{percent: 100, value: 20},
		{percent: 125, value: 25},
		{percent: 150, value: 30},
		{percent: 200, value: 40},
	}
	for _, test := range tests {
		t.Run(strconv.Itoa(test.percent), func(t *testing.T) {
			format := richEditParagraphFormat(test.percent)
			if format.CbSize != uint32(unsafe.Sizeof(format)) {
				t.Fatalf("cbSize = %d, want %d", format.CbSize, unsafe.Sizeof(format))
			}
			if format.DwMask != pfmLineSpacing {
				t.Fatalf("mask = %#x, want %#x", format.DwMask, pfmLineSpacing)
			}
			if format.BLineSpacingRule != 5 {
				t.Fatalf("line spacing rule = %d, want 5", format.BLineSpacingRule)
			}
			if format.DyLineSpacing != test.value {
				t.Fatalf(
					"line spacing value for %d%% = %d, want %d",
					test.percent,
					format.DyLineSpacing,
					test.value,
				)
			}
		})
	}
}
