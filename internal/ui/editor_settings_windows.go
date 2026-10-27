//go:build windows

package ui

import (
	"fmt"

	"github.com/nzai/MyNote/internal/config"
	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

func (d *Document) applyEditorSettings(settings config.EditorConfig) error {
	if err := settings.Validate(); err != nil {
		return fmt.Errorf("apply editor settings: %w", err)
	}
	if d.snapshot.Kind != core.KindText || d.edit == 0 {
		d.editorSettings = settings
		return nil
	}

	dpi := dpiFor(d.native.hwnd)
	nextFont, err := createUIFont(
		settings.FontPointSize,
		dpi,
		settings.FontFamily,
		int(co.FW_NORMAL),
	)
	if err != nil {
		return fmt.Errorf("apply editor settings: %w", err)
	}

	oldFont := d.editorFont
	_, _ = d.edit.SendMessage(co.WM_SETREDRAW, 0, 0)
	if _, err := d.edit.SendMessage(co.WM_SETFONT, win.WPARAM(nextFont), 1); err != nil {
		_ = nextFont.DeleteObject()
		_, _ = d.edit.SendMessage(co.WM_SETREDRAW, 1, 0)
		return fmt.Errorf("apply editor font handle: %w", err)
	}
	if err := configureRichEdit(d.edit, settings); err != nil {
		if oldFont != 0 {
			_, _ = d.edit.SendMessage(co.WM_SETFONT, win.WPARAM(oldFont), 1)
			_ = configureRichEdit(d.edit, d.editorSettings)
		}
		_ = nextFont.DeleteObject()
		_, _ = d.edit.SendMessage(co.WM_SETREDRAW, 1, 0)
		return err
	}

	d.editorFont = nextFont
	previousSettings := d.editorSettings
	d.editorSettings = settings
	if err := d.applyEditorGeometry(); err != nil {
		d.editorSettings = previousSettings
		return err
	}
	if oldFont != 0 {
		_ = oldFont.DeleteObject()
	}
	d.lineNumberFirstVisible = -1
	_, _ = d.edit.SendMessage(co.WM_SETREDRAW, 1, 0)
	_ = d.edit.InvalidateRect(nil, true)
	_ = d.native.hwnd.InvalidateRect(nil, true)
	return nil
}

func (d *Document) applyEditorGeometry() error {
	d.layoutEditor()
	d.updateScrollbars()
	d.invalidateLineNumberGutter()
	return nil
}

func (d *Document) lineNumbersVisible() bool {
	return d.snapshot.Kind == core.KindText &&
		d.edit != 0 &&
		d.editorSettings.ShowLineNumbers
}
