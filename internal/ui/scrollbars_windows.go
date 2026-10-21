//go:build windows

package ui

import (
	"fmt"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const scrollbarUpdatePasses = 4

type scrollbarVisibility struct {
	horizontal bool
	vertical   bool
}

func (d *Document) updateScrollbars() {
	if d.edit == 0 || d.updatingBars {
		return
	}
	d.updatingBars = true
	_, _ = d.edit.SendMessage(co.WM_SETREDRAW, 0, 0)
	defer func() {
		_, _ = d.edit.SendMessage(co.WM_SETREDRAW, 1, 0)
		_ = d.edit.InvalidateRect(nil, true)
		d.updatingBars = false
	}()

	// EDIT updates its scroll ranges only while a standard scroll bar is
	// visible. Start from both bars so a text change is measured before the
	// unneeded bars are hidden again.
	visibility := scrollbarVisibility{horizontal: true, vertical: true}
	d.setScrollbarVisibility(visibility)
	for pass := 0; pass < scrollbarUpdatePasses; pass++ {
		next := scrollbarVisibility{
			horizontal: d.scrollbarNeeded(co.SBB_HORZ),
			vertical:   d.scrollbarNeeded(co.SBB_VERT),
		}
		if next == visibility {
			return
		}
		visibility = next
		d.setScrollbarVisibility(visibility)
	}
}

func (d *Document) scrollbarNeeded(bar co.SBB) bool {
	var info win.SCROLLINFO
	info.Mask = co.SIF_RANGE | co.SIF_PAGE
	info.SetCbSize()
	if err := d.edit.GetScrollInfo(bar, &info); err != nil {
		d.logError("query editor scroll range", err)
		return false
	}
	return scrollRangeExceedsPage(info)
}

func scrollRangeExceedsPage(info win.SCROLLINFO) bool {
	if info.Page == 0 {
		return info.Max > info.Min
	}
	extent := int64(info.Max) - int64(info.Min) + 1
	return extent > int64(info.Page)
}

func (d *Document) setScrollbarVisibility(visibility scrollbarVisibility) {
	if d.scrollbars.horizontal != visibility.horizontal {
		if err := d.edit.ShowScrollBar(co.SBB_HORZ, visibility.horizontal); err != nil {
			d.logError("set horizontal scrollbar visibility", fmt.Errorf("show=%v: %w", visibility.horizontal, err))
		}
	}
	if d.scrollbars.vertical != visibility.vertical {
		if err := d.edit.ShowScrollBar(co.SBB_VERT, visibility.vertical); err != nil {
			d.logError("set vertical scrollbar visibility", fmt.Errorf("show=%v: %w", visibility.vertical, err))
		}
	}
	d.scrollbars = visibility
}
