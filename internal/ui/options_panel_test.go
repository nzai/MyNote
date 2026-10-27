//go:build windows

package ui

import (
	"errors"
	"os"
	"reflect"
	"syscall"
	"testing"
	"unsafe"

	"github.com/nzai/MyNote/internal/config"
	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

var isWindowEnabledProc = syscall.NewLazyDLL("user32.dll").NewProc("IsWindowEnabled")
var getUpdateRectProc = syscall.NewLazyDLL("user32.dll").NewProc("GetUpdateRect")
var getPixelProc = syscall.NewLazyDLL("gdi32.dll").NewProc("GetPixel")

func TestOptionsPanelAppliesLiveAndCancelRestoresSettings(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	document := newOptionsTestDocument(t)
	baseline := document.editorSettings

	if err := document.openOptions(); err != nil {
		t.Fatal(err)
	}
	panel := document.options
	if panel == nil || panel.combo == 0 {
		t.Fatal("options panel and font selector were not created")
	}
	background, err := brushFor(colorOptionsPanel)
	if err != nil || background == 0 {
		t.Fatalf("options background brush = %#v, %v", background, err)
	}
	hdc, err := panel.native.hwnd.GetDC()
	if err != nil {
		t.Fatal(err)
	}
	paintErr := panel.paint(hdc)
	_ = panel.native.hwnd.ReleaseDC(hdc)
	if paintErr != nil {
		t.Fatalf("paint options panel: %v", paintErr)
	}

	next := panel.settings
	next.FontFamily = "Consolas"
	next.FontPointSize = 18
	next.LineSpacing = 150
	next.ShowLineNumbers = false
	panel.applySettings(next)
	if panel.errorText != "" {
		t.Fatalf("apply settings error: %s", panel.errorText)
	}
	if !reflect.DeepEqual(document.editorSettings, next) {
		t.Fatalf("live document settings = %#v, want %#v", document.editorSettings, next)
	}

	document.closeOptions(true)
	if document.options != nil {
		t.Fatal("options panel must be removed after cancel")
	}
	if !reflect.DeepEqual(document.editorSettings, baseline) {
		t.Fatalf(
			"settings after cancel = %#v, want baseline %#v",
			document.editorSettings,
			baseline,
		)
	}
}

func TestOptionsHoverInvalidatesOnlyAffectedControl(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	document := newOptionsTestDocument(t)
	if err := document.openOptions(); err != nil {
		t.Fatal(err)
	}
	defer document.closeOptions(true)

	panel := document.options
	panel.native.hwnd.UpdateWindow()
	layout := panel.currentLayout()
	x := int(layout.autoStartCheckbox.Left) + 12
	y := int(layout.autoStartCheckbox.Top) + 8
	panel.handleMouseMove(mousePosition(x, y))

	update, err := windowUpdateRect(panel.native.hwnd)
	if err != nil {
		t.Fatal(err)
	}
	if update.Right <= update.Left || update.Bottom <= update.Top {
		t.Fatal("mouse move did not invalidate the hovered control")
	}
	if !rectContains(layout.autoStartCheckbox, update) {
		t.Fatalf(
			"hover update rect %+v escapes auto-start control %+v; the whole window was repainted",
			update,
			layout.autoStartCheckbox,
		)
	}
}

func TestOptionsPaintKeepsPreviousFrameVisibleUntilComplete(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	document := newOptionsTestDocument(t)
	if err := document.openOptions(); err != nil {
		t.Fatal(err)
	}
	defer document.closeOptions(true)

	panel := document.options
	layout := panel.currentLayout()
	point := win.POINT{
		X: layout.saveButton.Left + 12,
		Y: (layout.saveButton.Top + layout.saveButton.Bottom) / 2,
	}
	width, height, err := clientSize(panel.native.hwnd)
	if err != nil {
		t.Fatal(err)
	}
	screen, err := win.HWND(0).GetDC()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = win.HWND(0).ReleaseDC(screen)
	}()
	target, err := screen.CreateCompatibleDC()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = target.DeleteDC()
	}()
	targetBitmap, err := screen.CreateCompatibleBitmap(width, height)
	if err != nil {
		t.Fatal(err)
	}
	oldTargetBitmap, err := target.SelectObjectBmp(targetBitmap)
	if err != nil {
		_ = targetBitmap.DeleteObject()
		t.Fatal(err)
	}
	defer func() {
		_, _ = target.SelectObjectBmp(oldTargetBitmap)
		_ = targetBitmap.DeleteObject()
	}()

	if err := panel.paintFrame(target); err != nil {
		t.Fatal(err)
	}
	before, err := pixelColor(target, point)
	if err != nil {
		t.Fatal(err)
	}
	if before != colorOptionsAccent {
		t.Fatalf("save button pixel = %#08x, want complete-frame accent %#08x", before, colorOptionsAccent)
	}

	var targetDuringPaint win.COLORREF
	var bufferDuringPaint win.COLORREF
	panel.paintObserver = func(target, draw win.HDC) {
		targetDuringPaint, _ = pixelColor(target, point)
		bufferDuringPaint, _ = pixelColor(draw, point)
	}
	defer func() {
		panel.paintObserver = nil
	}()
	if err := panel.paintFrame(target); err != nil {
		t.Fatal(err)
	}

	if targetDuringPaint != before {
		t.Fatalf(
			"target pixel changed to %#08x before the new frame was complete; want previous frame %#08x",
			targetDuringPaint,
			before,
		)
	}
	if bufferDuringPaint != colorOptionsPanel {
		t.Fatalf(
			"offscreen pixel after background = %#08x, want options background %#08x",
			bufferDuringPaint,
			colorOptionsPanel,
		)
	}
	after, err := pixelColor(target, point)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("final target pixel = %#08x, want %#08x", after, before)
	}
}

func TestOptionsWindowPaintUsesOffscreenFrame(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	document := newOptionsTestDocument(t)
	if err := document.openOptions(); err != nil {
		t.Fatal(err)
	}
	defer document.closeOptions(true)

	panel := document.options
	panel.native.hwnd.UpdateWindow()
	layout := panel.currentLayout()
	point := win.POINT{
		X: layout.saveButton.Left + 12,
		Y: (layout.saveButton.Top + layout.saveButton.Bottom) / 2,
	}
	var targetHdc win.HDC
	var drawHdc win.HDC
	var drawPixel win.COLORREF
	panel.paintObserver = func(target, draw win.HDC) {
		targetHdc = target
		drawHdc = draw
		drawPixel, _ = pixelColor(draw, point)
	}
	defer func() {
		panel.paintObserver = nil
	}()
	_ = panel.native.hwnd.InvalidateRect(nil, true)
	panel.native.hwnd.UpdateWindow()

	if targetHdc == 0 || drawHdc == 0 {
		t.Fatal("WM_PAINT did not reach the instrumented paint path")
	}
	if targetHdc == drawHdc {
		t.Fatal("WM_PAINT drew directly to the visible target DC")
	}
	if drawPixel != colorOptionsPanel {
		t.Fatalf(
			"offscreen pixel after background = %#08x, want options background %#08x",
			drawPixel,
			colorOptionsPanel,
		)
	}
}

func TestOptionsFooterButtonLabelsAreHorizontallyCentered(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	document := newOptionsTestDocument(t)
	if err := document.openOptions(); err != nil {
		t.Fatal(err)
	}
	defer document.closeOptions(true)

	panel := document.options
	width, height, err := clientSize(panel.native.hwnd)
	if err != nil {
		t.Fatal(err)
	}
	screen, err := win.HWND(0).GetDC()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = win.HWND(0).ReleaseDC(screen)
	}()
	target, err := screen.CreateCompatibleDC()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = target.DeleteDC()
	}()
	bitmap, err := screen.CreateCompatibleBitmap(width, height)
	if err != nil {
		t.Fatal(err)
	}
	oldBitmap, err := target.SelectObjectBmp(bitmap)
	if err != nil {
		_ = bitmap.DeleteObject()
		t.Fatal(err)
	}
	defer func() {
		_, _ = target.SelectObjectBmp(oldBitmap)
		_ = bitmap.DeleteObject()
	}()
	if err := panel.paintFrame(target); err != nil {
		t.Fatal(err)
	}

	layout := panel.currentLayout()
	dpi := dpiFor(panel.native.hwnd)
	tests := []struct {
		name       string
		rect       win.RECT
		background win.COLORREF
	}{
		{
			name:       "cancel",
			rect:       layout.cancelButton,
			background: colorWindowBackground,
		},
		{
			name:       "save",
			rect:       layout.saveButton,
			background: colorOptionsAccent,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			minX, _, maxX, _, pixels, err := textInkBounds(
				target,
				test.rect,
				test.background,
				scaleDIP(10, dpi),
				scaleDIP(5, dpi),
			)
			if err != nil {
				t.Fatal(err)
			}
			if pixels == 0 {
				t.Fatal("button label has no visible pixels")
			}
			inkCenterX := float64(minX+maxX+1) / 2
			buttonCenterX := float64(test.rect.Left+test.rect.Right) / 2
			if delta := absFloat(inkCenterX - buttonCenterX); delta > 2 {
				t.Fatalf(
					"button label center x = %.1f, button center x = %.1f, delta = %.1f",
					inkCenterX,
					buttonCenterX,
					delta,
				)
			}
		})
	}
}

func TestOptionsTextRightAlignsCJKText(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	document := newOptionsTestDocument(t)
	if err := document.openOptions(); err != nil {
		t.Fatal(err)
	}
	defer document.closeOptions(true)

	panel := document.options
	screen, err := win.HWND(0).GetDC()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = win.HWND(0).ReleaseDC(screen)
	}()
	target, err := screen.CreateCompatibleDC()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = target.DeleteDC()
	}()
	bitmap, err := screen.CreateCompatibleBitmap(220, 50)
	if err != nil {
		t.Fatal(err)
	}
	oldBitmap, err := target.SelectObjectBmp(bitmap)
	if err != nil {
		_ = bitmap.DeleteObject()
		t.Fatal(err)
	}
	defer func() {
		_, _ = target.SelectObjectBmp(oldBitmap)
		_ = bitmap.DeleteObject()
	}()
	background, err := brushFor(colorWindowBackground)
	if err != nil {
		t.Fatal(err)
	}
	fullRect := win.RECT{Right: 220, Bottom: 50}
	if err := target.FillRect(&fullRect, background); err != nil {
		t.Fatal(err)
	}

	rect := win.RECT{Left: 10, Top: 5, Right: 210, Bottom: 45}
	if err := paintOptionsText(
		target,
		rect,
		"取消",
		panel.document.fonts.ui,
		colorTextPrimary,
		co.DT_RIGHT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_NOPREFIX,
	); err != nil {
		t.Fatal(err)
	}
	minX, _, maxX, _, pixels, err := textInkBounds(
		target,
		rect,
		colorWindowBackground,
		0,
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	if pixels == 0 {
		t.Fatal("right-aligned CJK text has no visible pixels")
	}
	if maxX < int(rect.Right)-4 {
		t.Fatalf(
			"right-aligned CJK ink ends at x=%d, want near right edge x=%d (starts at x=%d)",
			maxX,
			rect.Right,
			minX,
		)
	}
}

func TestOptionsTitlebarButtonOpensOwnedTopLevelWindow(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	document := newOptionsTestDocument(t)
	width, height, err := clientSize(document.native.hwnd)
	if err != nil {
		t.Fatal(err)
	}
	dpi := dpiFor(document.native.hwnd)
	layout := titlebarLayoutFor(width, dpi, document.titleTextWidth)
	x := (layout.optionsLeft + layout.optionsRight) / 2
	y := scaleDIP(titlebarHeightDIP/2, dpi)
	ownerBefore, err := document.native.hwnd.GetWindowRect()
	if err != nil {
		t.Fatal(err)
	}

	document.handleClick(mousePosition(x, y))

	if document.options == nil || document.options.native.hwnd == 0 {
		t.Fatal("clicking the options titlebar button did not open the options window")
	}
	dialog := document.options.native.hwnd
	if !dialog.IsWindowVisible() {
		t.Fatal("options window was created but is not visible")
	}
	style, err := dialog.GetWindowLongPtr(co.GWLP_STYLE)
	if err != nil {
		t.Fatal(err)
	}
	windowStyle := co.WS(style)
	if windowStyle&co.WS_CHILD != 0 {
		t.Fatal("options window must not be a WS_CHILD control")
	}
	if windowStyle&co.WS_POPUP == 0 ||
		windowStyle&co.WS_CAPTION == 0 ||
		windowStyle&co.WS_SYSMENU == 0 {
		t.Fatalf("options window style %#x lacks popup/caption/system menu", style)
	}
	if windowStyle&co.WS_THICKFRAME != 0 {
		t.Fatal("options window must use a fixed dialog frame")
	}
	exStyle, err := dialog.GetWindowLongPtr(co.GWLP_EXSTYLE)
	if err != nil {
		t.Fatal(err)
	}
	if co.WS_EX(exStyle)&co.WS_EX_DLGMODALFRAME == 0 {
		t.Fatalf("options window ex-style %#x lacks dialog modal frame", exStyle)
	}
	owner, err := dialog.GetWindow(co.GW_OWNER)
	if err != nil {
		t.Fatal(err)
	}
	if owner != document.native.hwnd {
		t.Fatalf("options window owner = %#x, want document %#x", owner, document.native.hwnd)
	}
	rect, err := dialog.GetWindowRect()
	if err != nil {
		t.Fatal(err)
	}
	if rect.Right <= rect.Left || rect.Bottom <= rect.Top {
		t.Fatalf("options window has empty bounds: %+v", rect)
	}
	ownerAfter, err := document.native.hwnd.GetWindowRect()
	if err != nil {
		t.Fatal(err)
	}
	if ownerAfter != ownerBefore {
		t.Fatalf(
			"document bounds changed from %+v to %+v while opening options",
			ownerBefore,
			ownerAfter,
		)
	}
	if y < 0 || y >= height {
		t.Fatalf("test click y=%d is outside client height %d", y, height)
	}
}

func TestOptionsWindowIsCenteredOverDocument(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	document := newOptionsTestDocument(t)
	if err := document.openOptions(); err != nil {
		t.Fatal(err)
	}
	defer document.closeOptions(true)

	owner, err := document.native.hwnd.GetWindowRect()
	if err != nil {
		t.Fatal(err)
	}
	dialog, err := document.options.native.hwnd.GetWindowRect()
	if err != nil {
		t.Fatal(err)
	}
	ownerCenterX := (owner.Left + owner.Right) / 2
	ownerCenterY := (owner.Top + owner.Bottom) / 2
	dialogCenterX := (dialog.Left + dialog.Right) / 2
	dialogCenterY := (dialog.Top + dialog.Bottom) / 2
	if absInt(int(dialogCenterX-ownerCenterX)) > 2 ||
		absInt(int(dialogCenterY-ownerCenterY)) > 2 {
		t.Fatalf(
			"options window center = (%d,%d), owner center = (%d,%d)",
			dialogCenterX,
			dialogCenterY,
			ownerCenterX,
			ownerCenterY,
		)
	}
}

func TestOptionsWindowDisablesAndRestoresOwner(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	document := newOptionsTestDocument(t)
	if !isWindowEnabled(document.native.hwnd) {
		t.Fatal("document window must be enabled before opening options")
	}
	if err := document.openOptions(); err != nil {
		t.Fatal(err)
	}
	if isWindowEnabled(document.native.hwnd) {
		t.Fatal("document owner must be disabled while the options window is open")
	}

	document.closeOptions(true)
	if document.options != nil {
		t.Fatal("options window must be removed after cancel")
	}
	if !isWindowEnabled(document.native.hwnd) {
		t.Fatal("document owner must be enabled again after closing options")
	}
}

func TestOptionsSystemCloseRestoresOwner(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	document := newOptionsTestDocument(t)
	if err := document.openOptions(); err != nil {
		t.Fatal(err)
	}
	if _, err := document.options.native.hwnd.SendMessage(co.WM_CLOSE, 0, 0); err != nil {
		t.Fatal(err)
	}
	if document.options != nil {
		t.Fatal("options window must be removed after a system close")
	}
	if !isWindowEnabled(document.native.hwnd) {
		t.Fatal("document owner must be enabled again after a system close")
	}
}

func TestOptionsPanelPersistsOnlyWhenSaveSucceeds(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	var savedSettings config.EditorConfig
	var savedAutoStart bool
	document := newOptionsTestDocument(t)
	document.saveSettings = func(settings config.EditorConfig, autoStart bool) error {
		savedSettings = settings
		savedAutoStart = autoStart
		return nil
	}
	if err := document.openOptions(); err != nil {
		t.Fatal(err)
	}
	panel := document.options
	next := panel.settings
	next.FontPointSize = 20
	next.SuppressClosePrompt = true
	panel.applySettings(next)
	panel.autoStart = true
	panel.save()

	if document.options != nil {
		t.Fatal("options panel must close after a successful save")
	}
	if !reflect.DeepEqual(savedSettings, next) {
		t.Fatalf("persisted settings = %#v, want %#v", savedSettings, next)
	}
	if !savedAutoStart {
		t.Fatal("auto-start value was not persisted")
	}
	if !reflect.DeepEqual(document.editorSettingsBaseline, next) {
		t.Fatalf("baseline = %#v, want %#v", document.editorSettingsBaseline, next)
	}
}

func TestOptionsPanelStaysOpenAndShowsInlineErrorWhenSaveFails(t *testing.T) {
	if os.Getenv("CLIPNOTE_GUI_TEST") == "" {
		t.Skip("set CLIPNOTE_GUI_TEST=1 for Win32 options-panel test")
	}
	saveErr := errors.New("config is read-only")
	document := newOptionsTestDocument(t)
	document.saveSettings = func(config.EditorConfig, bool) error {
		return saveErr
	}
	if err := document.openOptions(); err != nil {
		t.Fatal(err)
	}
	panel := document.options
	panel.save()

	if document.options != panel {
		t.Fatal("options panel must remain open after save failure")
	}
	if panel.errorText != saveErr.Error() {
		t.Fatalf("inline error = %q, want %q", panel.errorText, saveErr.Error())
	}
}

func newOptionsTestDocument(t *testing.T) *Document {
	t.Helper()
	lockGUIThread(t)
	document, err := NewDocument(DocumentOptions{
		Snapshot: core.Snapshot{
			CaptureID: "options-panel-test",
			Kind:      core.KindText,
			Text:      "options panel preview",
		},
		EditorSettings: config.DefaultEditorConfig(),
		FontFamilies:   []string{"Consolas", "Microsoft YaHei UI", "Segoe UI"},
		Position:       win.POINT{X: -20000, Y: -20000},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = document.native.hwnd.ShowWindow(co.SW_SHOWNOACTIVATE)
	t.Cleanup(func() {
		_ = document.native.hwnd.DestroyWindow()
	})
	return document
}

func isWindowEnabled(hwnd win.HWND) bool {
	ret, _, _ := isWindowEnabledProc.Call(uintptr(hwnd))
	return ret != 0
}

func windowUpdateRect(hwnd win.HWND) (win.RECT, error) {
	var rect win.RECT
	ret, _, err := getUpdateRectProc.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(&rect)),
		0,
	)
	if ret == 0 {
		return win.RECT{}, err
	}
	return rect, nil
}

func pixelColor(hdc win.HDC, point win.POINT) (win.COLORREF, error) {
	ret, _, err := getPixelProc.Call(
		uintptr(hdc),
		uintptr(point.X),
		uintptr(point.Y),
	)
	const clrInvalid = 0xFFFFFFFF
	if ret == clrInvalid {
		return 0, err
	}
	return win.COLORREF(ret), nil
}

func textInkBounds(
	hdc win.HDC,
	rect win.RECT,
	background win.COLORREF,
	insetX, insetY int,
) (int, int, int, int, int, error) {
	minX, minY := int(^uint(0)>>1), int(^uint(0)>>1)
	maxX, maxY := -1, -1
	pixels := 0
	for y := int(rect.Top) + insetY; y < int(rect.Bottom)-insetY; y++ {
		for x := int(rect.Left) + insetX; x < int(rect.Right)-insetX; x++ {
			color, err := pixelColor(hdc, win.POINT{X: int32(x), Y: int32(y)})
			if err != nil {
				return 0, 0, 0, 0, 0, err
			}
			if color == background {
				continue
			}
			minX = min(minX, x)
			minY = min(minY, y)
			maxX = max(maxX, x)
			maxY = max(maxY, y)
			pixels++
		}
	}
	return minX, minY, maxX, maxY, pixels, nil
}

func rectContains(outer, inner win.RECT) bool {
	return inner.Left >= outer.Left &&
		inner.Top >= outer.Top &&
		inner.Right <= outer.Right &&
		inner.Bottom <= outer.Bottom
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
