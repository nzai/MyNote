//go:build windows

package ui

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"github.com/nzai/MyNote/internal/config"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const (
	optionsPanelClass       = "ClipNote.OptionsPanel"
	optionsFontControlID    = 201
	optionsSliderPaddingDIP = 8
	optionsCheckboxSizeDIP  = 17
	optionsButtonRadiusDIP  = 5
)

var (
	colorOptionsPanel        = colorRef(252, 253, 252)
	colorOptionsPanelBorder  = colorRef(194, 205, 199)
	colorOptionsPreview      = colorRef(243, 248, 246)
	colorOptionsAccent       = colorRef(26, 112, 104)
	colorOptionsAccentStrong = colorRef(20, 94, 87)
	colorOptionsAccentSoft   = colorRef(226, 240, 236)
	colorOptionsTrack        = colorRef(211, 220, 216)
	colorOptionsError        = colorRef(184, 42, 38)
)

type optionsPanel struct {
	native   nativeWindowState
	document *Document

	settings          config.EditorConfig
	baselineSettings  config.EditorConfig
	autoStart         bool
	baselineAutoStart bool
	fontFamilies      []string
	combo             win.HWND
	previewFont       win.HFONT
	selectorFont      win.HFONT
	hover             optionsPanelHit
	pressed           optionsPanelHit
	dragging          optionsPanelHit
	capturing         bool
	errorText         string
	destroyed         bool
	ownerDisabled     bool
	// The Win32 flicker regression test samples the target and offscreen DCs at
	// the same paint stage; production leaves this hook nil.
	paintObserver func(target, draw win.HDC)
}

func (d *Document) toggleOptions() {
	if d.options != nil {
		d.closeOptions(true)
		return
	}
	if err := d.openOptions(); err != nil {
		d.logError("open editor options", err)
		d.saveStatus = "无法打开编辑选项，详情见日志"
		_ = d.native.hwnd.InvalidateRect(nil, true)
	}
}

func (d *Document) openOptions() error {
	if d.native.hwnd == 0 || d.destroyed {
		return errors.New("open editor options: document window is not available")
	}
	ownerRect, err := d.native.hwnd.GetWindowRect()
	if err != nil {
		return fmt.Errorf("measure document window before opening options: %w", err)
	}
	dpi := dpiFor(d.native.hwnd)
	size := win.SIZE{
		Cx: int32(scaleDIP(optionsWindowWidthDIP, dpi)),
		Cy: int32(scaleDIP(optionsWindowHeightDIP, dpi)),
	}
	position := win.POINT{
		X: ownerRect.Left + (ownerRect.Right-ownerRect.Left-size.Cx)/2,
		Y: ownerRect.Top + (ownerRect.Bottom-ownerRect.Top-size.Cy)/2,
	}
	class, err := registerNativeClass(optionsPanelClass)
	if err != nil {
		return err
	}
	panel := &optionsPanel{
		document:          d,
		settings:          d.editorSettings,
		baselineSettings:  d.editorSettingsBaseline,
		autoStart:         d.autoStart,
		baselineAutoStart: d.autoStart,
		fontFamilies: ensureFontFamily(
			d.fontFamilies,
			d.editorSettings.FontFamily,
		),
	}
	panel.native.handler = panel
	// An owned top-level window with the owner disabled is a native modal
	// dialog; a child window would reintroduce RichEdit Z-order coupling.
	hwnd, err := createNativeWindow(
		class,
		co.WS_EX_DLGMODALFRAME,
		co.WS_POPUP|co.WS_CAPTION|co.WS_SYSMENU|co.WS_CLIPCHILDREN,
		"编辑选项",
		position,
		size,
		d.native.hwnd,
		&panel.native,
	)
	if err != nil {
		return err
	}
	panel.native.hwnd = hwnd
	if d.icon != 0 {
		if err := applyWindowIcon(hwnd, d.icon); err != nil {
			_ = hwnd.DestroyWindow()
			return err
		}
	}
	d.options = panel
	if err := panel.initialize(); err != nil {
		d.options = nil
		_ = hwnd.DestroyWindow()
		return err
	}
	panel.layoutChildren(panel.currentLayout())
	// Disable the owner before mapping the dialog so no click can bypass it.
	d.native.hwnd.EnableWindow(false)
	panel.ownerDisabled = true
	hwnd.ShowWindow(co.SW_SHOW)
	hwnd.UpdateWindow()
	_ = hwnd.SetForegroundWindow()
	_, _ = hwnd.SetFocus()
	_ = d.native.hwnd.InvalidateRect(nil, true)
	return nil
}

func (d *Document) closeOptions(cancel bool) {
	panel := d.options
	if panel == nil {
		return
	}
	d.options = nil
	if cancel {
		if err := d.applyEditorSettings(panel.baselineSettings); err != nil {
			d.logError("restore editor settings after closing options", err)
			d.saveStatus = "恢复编辑设置失败，详情见日志"
		}
		d.autoStart = panel.baselineAutoStart
	}
	if err := panel.native.hwnd.DestroyWindow(); err != nil {
		d.logError("destroy editor options window", err)
	}
	panel.releaseOwner()
	if d.edit != 0 {
		_, _ = d.edit.SetFocus()
	} else {
		_, _ = d.native.hwnd.SetFocus()
	}
	_ = d.native.hwnd.InvalidateRect(nil, true)
}

func (d *Document) restoreOptionsOwner() {
	if d.native.hwnd == 0 || d.destroyed || !d.native.hwnd.IsWindow() {
		return
	}
	d.native.hwnd.EnableWindow(true)
}

func (p *optionsPanel) initialize() error {
	dpi := dpiFor(p.native.hwnd)
	combo, err := win.CreateWindowEx(
		0,
		win.ClassNameStr("COMBOBOX"),
		"",
		co.WS_CHILD|co.WS_VISIBLE|co.WS_TABSTOP|co.WS_VSCROLL|
			co.WS(co.CBS_DROPDOWNLIST|co.CBS_HASSTRINGS),
		win.POINT{},
		win.SIZE{},
		p.native.hwnd,
		win.HMENU(optionsFontControlID),
		win.HINSTANCE(0),
		win.LPARAM(0),
	)
	if err != nil {
		return fmt.Errorf("create font selector: %w", err)
	}
	p.combo = combo
	for _, family := range p.fontFamilies {
		if err := comboAddString(combo, family); err != nil {
			return err
		}
	}
	selected := fontFamilyIndex(p.fontFamilies, p.settings.FontFamily)
	if selected < 0 {
		return fmt.Errorf("font family %q is missing from selector", p.settings.FontFamily)
	}
	if _, err := combo.SendMessage(
		co.CB_SETCURSEL,
		win.WPARAM(selected),
		0,
	); err != nil {
		return fmt.Errorf("select current font family: %w", err)
	}
	if err := p.updateFontResources(dpi); err != nil {
		return err
	}
	p.layoutChildren(p.currentLayout())
	return nil
}

func (p *optionsPanel) layoutChildren(layout optionsPanelLayout) {
	if p.combo == 0 {
		return
	}
	rect := layout.fontSelector
	dpi := dpiFor(p.native.hwnd)
	_ = p.combo.MoveWindow(
		win.POINT{X: rect.Left, Y: rect.Top},
		win.SIZE{
			Cx: rect.Right - rect.Left,
			Cy: int32(scaleDIP(240, dpi)),
		},
		true,
	)
}

func (p *optionsPanel) updateFontResources(dpi int) error {
	previewFont, err := createUIFont(
		p.settings.FontPointSize,
		dpi,
		p.settings.FontFamily,
		int(co.FW_NORMAL),
	)
	if err != nil {
		return err
	}
	selectorPointSize := p.settings.FontPointSize
	if selectorPointSize < 10 {
		selectorPointSize = 10
	}
	if selectorPointSize > 14 {
		selectorPointSize = 14
	}
	selectorFont, err := createUIFont(
		selectorPointSize,
		dpi,
		p.settings.FontFamily,
		int(co.FW_NORMAL),
	)
	if err != nil {
		_ = previewFont.DeleteObject()
		return err
	}

	oldPreview := p.previewFont
	oldSelector := p.selectorFont
	p.previewFont = previewFont
	p.selectorFont = selectorFont
	if p.combo != 0 {
		if _, err := p.combo.SendMessage(
			co.WM_SETFONT,
			win.WPARAM(selectorFont),
			1,
		); err != nil {
			p.previewFont = oldPreview
			p.selectorFont = oldSelector
			_ = previewFont.DeleteObject()
			_ = selectorFont.DeleteObject()
			return fmt.Errorf("apply font selector preview font: %w", err)
		}
	}
	if oldPreview != 0 {
		_ = oldPreview.DeleteObject()
	}
	if oldSelector != 0 {
		_ = oldSelector.DeleteObject()
	}
	return nil
}

func (p *optionsPanel) handleNativeMessage(
	msg co.WM,
	wParam win.WPARAM,
	lParam win.LPARAM,
) (uintptr, bool) {
	switch msg {
	case co.WM_ERASEBKGND:
		return 1, true
	case co.WM_PAINT:
		return p.handlePaint()
	case co.WM_PRINTCLIENT:
		if err := p.paint(win.HDC(wParam)); err != nil {
			if p.document != nil {
				p.document.logError("print editor options client", err)
			}
			return 0, false
		}
		return 0, true
	case co.WM_PRINT:
		if err := p.paint(win.HDC(wParam)); err != nil {
			if p.document != nil {
				p.document.logError("print editor options", err)
			}
			return 0, false
		}
		return 0, true
	case co.WM_SIZE:
		p.layoutChildren(p.currentLayout())
		_ = p.native.hwnd.InvalidateRect(nil, true)
		return 0, false
	case co.WM_DPICHANGED:
		p.handleDPIChanged(lParam)
		return 0, true
	case co.WM_MOUSEMOVE:
		p.handleMouseMove(lParam)
		return 0, true
	case co.WM_LBUTTONDOWN:
		p.handleMouseDown(lParam)
		return 0, true
	case co.WM_LBUTTONUP:
		p.handleMouseUp(lParam)
		return 0, true
	case co.WM_CAPTURECHANGED:
		p.dragging = optionsHitNone
		p.pressed = optionsHitNone
		p.capturing = false
		return 0, true
	case co.WM_COMMAND:
		p.handleCommand(wParam)
		return 0, true
	case co.WM_KEYDOWN:
		return p.handleKeyDown(wParam)
	case co.WM_CLOSE:
		p.document.closeOptions(true)
		return 0, true
	case co.WM_CTLCOLORLISTBOX:
		hdc := win.HDC(wParam)
		_, _ = hdc.SetTextColor(colorTextPrimary)
		_, _ = hdc.SetBkColor(colorOptionsPanel)
		brush, err := brushFor(colorOptionsPanel)
		if err != nil {
			return 0, false
		}
		return uintptr(brush), true
	case co.WM_NCDESTROY:
		p.handleDestroyed()
	}
	return 0, false
}

func (p *optionsPanel) handlePaint() (uintptr, bool) {
	var paint win.PAINTSTRUCT
	hdc, err := p.native.hwnd.BeginPaint(&paint)
	if err != nil {
		return 0, false
	}
	defer p.native.hwnd.EndPaint(&paint)
	if err := p.paintFrame(hdc); err != nil {
		if p.document != nil {
			p.document.logError("paint editor options", err)
		}
		return 0, false
	}
	return 0, true
}

func (p *optionsPanel) handleMouseMove(lParam win.LPARAM) {
	x, y := decodeMousePosition(uintptr(lParam))
	if p.dragging != optionsHitNone {
		p.updateSliderFromPointer(p.dragging, x)
		return
	}
	layout := p.currentLayout()
	next := optionsPanelHitAt(layout, x, y)
	if next != p.hover {
		previous := p.hover
		p.hover = next
		// Hover changes affect only the old and new control. Repainting the
		// whole dialog here makes the native child controls flicker under
		// continuous mouse movement.
		for _, rect := range optionsHoverInvalidRects(layout, previous, next) {
			_ = p.native.hwnd.InvalidateRect(&rect, false)
		}
	}
}

func (p *optionsPanel) handleMouseDown(lParam win.LPARAM) {
	x, y := decodeMousePosition(uintptr(lParam))
	hit := optionsPanelHitAt(p.currentLayout(), x, y)
	p.pressed = hit
	p.hover = hit
	switch hit {
	case optionsHitSizeSlider, optionsHitSpacingSlider:
		p.dragging = hit
		p.updateSliderFromPointer(hit, x)
	}
	if hit != optionsHitNone && !p.capturing {
		p.native.hwnd.SetCapture()
		p.capturing = true
	}
	_ = p.native.hwnd.InvalidateRect(nil, true)
}

func (p *optionsPanel) handleMouseUp(lParam win.LPARAM) {
	x, y := decodeMousePosition(uintptr(lParam))
	hit := optionsPanelHitAt(p.currentLayout(), x, y)
	wasPressed := p.pressed
	wasDragging := p.dragging
	p.pressed = optionsHitNone
	p.dragging = optionsHitNone
	if p.capturing {
		_ = win.ReleaseCapture()
		p.capturing = false
	}
	if wasDragging != optionsHitNone {
		_ = p.native.hwnd.InvalidateRect(nil, true)
		return
	}
	if wasPressed == optionsHitNone || hit != wasPressed {
		_ = p.native.hwnd.InvalidateRect(nil, true)
		return
	}
	switch hit {
	case optionsHitCancel:
		p.document.closeOptions(true)
	case optionsHitSave:
		p.save()
	case optionsHitAutoStart:
		p.autoStart = !p.autoStart
		p.errorText = ""
		_ = p.native.hwnd.InvalidateRect(nil, true)
	case optionsHitSuppressPrompt:
		next := p.settings
		next.SuppressClosePrompt = !next.SuppressClosePrompt
		p.applySettings(next)
	case optionsHitLineNumbers:
		next := p.settings
		next.ShowLineNumbers = !next.ShowLineNumbers
		p.applySettings(next)
	}
}

func (p *optionsPanel) handleCommand(wParam win.WPARAM) {
	if uint16(wParam) != optionsFontControlID {
		return
	}
	notification := uint16(wParam >> 16)
	if notification != uint16(co.CBN_SELCHANGE) {
		return
	}
	selected, err := p.combo.SendMessage(co.CB_GETCURSEL, 0, 0)
	if err != nil || int(selected) < 0 || int(selected) >= len(p.fontFamilies) {
		return
	}
	next := p.settings
	next.FontFamily = p.fontFamilies[int(selected)]
	p.applySettings(next)
	// Returning focus to the panel keeps Esc/Ctrl+Enter handling active after
	// the native combo box closes its popup.
	_, _ = p.native.hwnd.SetFocus()
}

func (p *optionsPanel) handleKeyDown(wParam win.WPARAM) (uintptr, bool) {
	switch co.VK(wParam) {
	case co.VK_ESCAPE:
		p.document.closeOptions(true)
		return 0, true
	case co.VK_RETURN:
		p.save()
		return 0, true
	default:
		return 0, false
	}
}

func (p *optionsPanel) handleDPIChanged(lParam win.LPARAM) {
	suggested := (*win.RECT)(pointerFromUintptr(uintptr(lParam)))
	if suggested == nil {
		return
	}
	if err := p.native.hwnd.MoveWindow(
		win.POINT{X: suggested.Left, Y: suggested.Top},
		win.SIZE{
			Cx: suggested.Right - suggested.Left,
			Cy: suggested.Bottom - suggested.Top,
		},
		true,
	); err != nil && p.document != nil {
		p.document.logError("resize editor options after DPI change", err)
	}
	if err := p.updateFontResources(dpiFor(p.native.hwnd)); err != nil &&
		p.document != nil {
		p.document.logError("update editor options fonts after DPI change", err)
	}
	p.layoutChildren(p.currentLayout())
	_ = p.native.hwnd.InvalidateRect(nil, true)
}

func (p *optionsPanel) applySettings(settings config.EditorConfig) {
	if p.document == nil {
		return
	}
	if err := p.document.applyEditorSettings(settings); err != nil {
		p.errorText = err.Error()
		_ = p.native.hwnd.InvalidateRect(nil, true)
		return
	}
	p.settings = settings
	p.errorText = ""
	if err := p.updateFontResources(dpiFor(p.native.hwnd)); err != nil {
		p.errorText = err.Error()
	}
	_ = p.native.hwnd.InvalidateRect(nil, true)
}

func (p *optionsPanel) save() {
	if p.document == nil {
		return
	}
	if p.document.saveSettings == nil {
		p.errorText = "设置保存服务未配置"
		_ = p.native.hwnd.InvalidateRect(nil, true)
		return
	}
	if err := p.document.saveSettings(p.settings, p.autoStart); err != nil {
		p.errorText = err.Error()
		_ = p.native.hwnd.InvalidateRect(nil, true)
		return
	}
	p.document.editorSettingsBaseline = p.settings
	p.document.autoStart = p.autoStart
	p.document.closeOptions(false)
}

func (p *optionsPanel) updateSliderFromPointer(slider optionsPanelHit, x int) {
	layout := p.currentLayout()
	var rect win.RECT
	switch slider {
	case optionsHitSizeSlider:
		rect = layout.sizeSlider
	case optionsHitSpacingSlider:
		rect = layout.spacingSlider
	default:
		return
	}
	left, right := sliderTrackRange(rect, dpiFor(p.native.hwnd))
	next := p.settings
	if slider == optionsHitSizeSlider {
		next.FontPointSize = sliderIntegerValue(
			x,
			left,
			right,
			config.MinEditorFontPointSize,
			config.MaxEditorFontPointSize,
		)
	} else {
		index := sliderIntegerValue(x, left, right, 0, len(config.LineSpacingOptions())-1)
		next.LineSpacing = config.LineSpacingOptions()[index]
	}
	if next != p.settings {
		p.applySettings(next)
	}
}

func (p *optionsPanel) handleDestroyed() {
	if p.destroyed {
		return
	}
	p.destroyed = true
	if p.document != nil && p.document.options == p {
		p.document.options = nil
		p.releaseOwner()
	}
	if p.previewFont != 0 {
		_ = p.previewFont.DeleteObject()
		p.previewFont = 0
	}
	if p.selectorFont != 0 {
		_ = p.selectorFont.DeleteObject()
		p.selectorFont = 0
	}
	p.combo = 0
}

func (p *optionsPanel) releaseOwner() {
	if !p.ownerDisabled {
		return
	}
	p.ownerDisabled = false
	if p.document != nil {
		p.document.restoreOptionsOwner()
	}
}

func (p *optionsPanel) currentLayout() optionsPanelLayout {
	if p.native.hwnd == 0 {
		return optionsPanelLayout{}
	}
	width, height, err := clientSize(p.native.hwnd)
	if err != nil {
		return optionsPanelLayout{}
	}
	return optionsPanelContentsLayout(
		win.RECT{Right: int32(width), Bottom: int32(height)},
		dpiFor(p.native.hwnd),
	)
}

func comboAddString(combo win.HWND, value string) error {
	ptr, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		return fmt.Errorf("encode font family %q: %w", value, err)
	}
	result, err := combo.SendMessage(
		co.CB_ADDSTRING,
		0,
		win.LPARAM(uintptr(unsafe.Pointer(ptr))),
	)
	if err != nil {
		return fmt.Errorf("add font family %q to selector: %w", value, err)
	}
	if int64(result) < 0 {
		return fmt.Errorf("add font family %q to selector: combo box rejected it", value)
	}
	return nil
}

func fontFamilyIndex(families []string, selected string) int {
	for index, family := range families {
		if strings.EqualFold(family, selected) {
			return index
		}
	}
	return -1
}

func sliderTrackRange(rect win.RECT, dpi int) (int, int) {
	padding := scaleDIP(optionsSliderPaddingDIP, dpi)
	return int(rect.Left) + padding, int(rect.Right) - padding
}
