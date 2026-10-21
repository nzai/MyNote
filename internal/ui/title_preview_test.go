//go:build windows

package ui

import (
	"strings"
	"testing"

	"github.com/nzai/MyNote/internal/core"
)

func TestDocumentPreviewTitle(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		imageWidth  int
		imageHeight int
		dirty       bool
		want        string
	}{
		{
			name: "collapses whitespace",
			text: "  第一行\r\n\t第二行   第三行  ",
			want: "第一行 第二行 第三行",
		},
		{
			name: "truncates by runes",
			text: strings.Repeat("界", 32),
			want: strings.Repeat("界", 32),
		},
		{
			name: "adds ellipsis after limit",
			text: strings.Repeat("界", 33),
			want: strings.Repeat("界", 32) + "…",
		},
		{
			name: "does not end preview with padding space",
			text: strings.Repeat("界", 31) + " 尾",
			want: strings.Repeat("界", 31) + "…",
		},
		{
			name: "empty text",
			text: " \r\n\t ",
			want: "（空文本）",
		},
		{
			name:        "image",
			imageWidth:  1920,
			imageHeight: 1080,
			want:        "（图片）1920 × 1080",
		},
		{
			name:  "dirty marker",
			text:  "会议记录",
			dirty: true,
			want:  "会议记录 *",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind := core.KindText
			if test.imageWidth > 0 {
				kind = core.KindImage
			}
			got := documentPreviewTitle(
				kind,
				test.text,
				test.imageWidth,
				test.imageHeight,
				test.dirty,
			)
			if got != test.want {
				t.Fatalf("documentPreviewTitle() = %q, want %q", got, test.want)
			}
		})
	}
}
