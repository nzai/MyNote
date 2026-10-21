//go:build windows

package ui

import (
	"log/slog"

	"github.com/rodrigocfd/windigo/win"
)

func documentDwmAttributes() []win.DwmAttr {
	return []win.DwmAttr{
		win.DwmAttrBorderColor(win.COLORREF_DWMA_NONE),
	}
}

func applyDocumentDwmAttributes(hwnd win.HWND, logger *slog.Logger) {
	for _, attr := range documentDwmAttributes() {
		if err := hwnd.DwmSetWindowAttribute(attr); err != nil {
			// Windows 10 lacks DWMWA_BORDER_COLOR. The document also suppresses
			// legacy non-client painting, so this is an expected fallback path.
			if logger != nil {
				logger.Debug(
					"DWM border color unavailable; using legacy border suppression",
					"error", err,
				)
			}
		}
	}
}
