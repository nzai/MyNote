//go:build windows

package ui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/nzai/MyNote/internal/core"
)

const titlePreviewMaxRunes = 32

func documentPreviewTitle(
	kind core.Kind,
	text string,
	imageWidth, imageHeight int,
	dirty bool,
) string {
	title := documentContentTitle(kind, text, imageWidth, imageHeight)
	if dirty {
		title += " *"
	}
	return title
}

func documentContentTitle(
	kind core.Kind,
	text string,
	imageWidth, imageHeight int,
) string {
	if kind == core.KindImage {
		return fmt.Sprintf("（图片）%d × %d", imageWidth, imageHeight)
	}
	preview := normalizedTitlePreview(text)
	if preview == "" {
		return "（空文本）"
	}
	return preview
}

func normalizedTitlePreview(text string) string {
	var preview strings.Builder
	preview.Grow(titlePreviewMaxRunes * 4)
	count := 0
	pendingSpace := false
	truncated := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if preview.Len() > 0 {
				pendingSpace = true
			}
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		if count >= titlePreviewMaxRunes {
			truncated = true
			break
		}
		if pendingSpace {
			if count+1 >= titlePreviewMaxRunes {
				truncated = true
				break
			}
			preview.WriteByte(' ')
			count++
		}
		pendingSpace = false
		preview.WriteRune(r)
		count++
	}
	if truncated {
		preview.WriteString("…")
	}
	return preview.String()
}
