//go:build windows

package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nzai/MyNote/internal/core"
	"github.com/nzai/MyNote/internal/save"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const (
	editControlID = 100

	contentPaddingDIP    = 12
	windowWidthDIP       = 720
	windowHeightDIP      = 520
	minWindowWidthDIP    = 320
	minWindowHeightDIP   = 300
	titleIconGapDIP      = 10
	appIconWidthDIP      = 16
	titleTextGapDIP      = 10
	statusTextPaddingDIP = 12
	statusCancelWidthDIP = 56
	onePixelDIP          = 1
	imageCheckerSizeDIP  = 16
	saveProgressTimerID  = 2
	saveCompleteMessage  = co.WM_APP + 103
)

var (
	colorWindowBackground = colorRef(255, 255, 255)
	colorTitlebar         = colorRef(247, 248, 250)
	colorTitlebarHover    = colorRef(232, 236, 243)
	colorCloseHover       = colorRef(196, 43, 28)
	colorCloseHoverText   = colorRef(255, 255, 255)
	colorTitlebarBorder   = colorRef(220, 223, 228)
	colorStatusbar        = colorRef(249, 250, 252)
	colorTextPrimary      = colorRef(31, 35, 40)
	colorTextMuted        = colorRef(101, 109, 118)
	colorAccent           = colorRef(33, 99, 188)
	colorCheckerLight     = colorRef(247, 248, 250)
	colorCheckerDark      = colorRef(231, 234, 239)

	documentRegistry = struct {
		sync.RWMutex
		documents map[win.HWND]*Document
	}{
		documents: make(map[win.HWND]*Document),
	}
)

type DocumentSaveResult struct {
	Path string
	Text string
}

type DocumentSaveJob struct {
	Run func(context.Context) (DocumentSaveResult, error)
}

type DocumentSaveFunc func(parent win.HWND, saveAs bool) (DocumentSaveJob, error)

type DocumentOptions struct {
	Snapshot        core.Snapshot
	Position        win.POINT
	Save            DocumentSaveFunc
	OnDestroy       func(*Document)
	OnCloseCanceled func(*Document)
	OnCancelCache   func()
	Logger          *slog.Logger
	ProgressDelay   time.Duration
}

type Window interface {
	Hwnd() win.HWND
	Show()
	Close()
	SetCacheStatus(string)
}

type Document struct {
	native          nativeWindowState
	snapshot        core.Snapshot
	saveState       *core.SaveState
	save            DocumentSaveFunc
	onDestroy       func(*Document)
	onCloseCanceled func(*Document)
	cancelCache     func()
	logger          *slog.Logger

	edit            win.HWND
	fonts           documentFonts
	hover           titlebarButton
	tracking        bool
	cacheState      string
	cacheCancelable bool
	saveStatus      string
	saveCancelable  bool
	saveInProgress  bool
	saveProgress    progressState
	saveCancel      context.CancelFunc
	saveStarted     time.Time
	closeAfterSave  bool
	saveResultMu    sync.Mutex
	saveResult      *saveCompletion
	destroyed       bool
	image           imageRaster
}

type saveCompletion struct {
	result DocumentSaveResult
	err    error
}

var _ Window = (*Document)(nil)

func NewDocument(options DocumentOptions) (*Document, error) {
	if err := validateDocumentSnapshot(options.Snapshot); err != nil {
		return nil, err
	}
	var saveState *core.SaveState
	var raster imageRaster
	switch options.Snapshot.Kind {
	case core.KindText:
		saveState = core.NewSaveState(options.Snapshot.Text)
	case core.KindImage:
		if options.Snapshot.Image == nil {
			return nil, fmt.Errorf("create image document: decoded image is nil")
		}
		var err error
		raster, err = rasterizeImage(options.Snapshot.Image)
		if err != nil {
			return nil, fmt.Errorf("create image document: %w", err)
		}
		saveState = core.NewReadOnlySaveState()
	default:
		return nil, fmt.Errorf("create document: unsupported content kind %d", options.Snapshot.Kind)
	}
	if options.ProgressDelay <= 0 {
		options.ProgressDelay = 3 * time.Second
	}
	class, err := registerNativeClass(documentWindowClass)
	if err != nil {
		return nil, err
	}

	doc := &Document{
		snapshot:        options.Snapshot,
		saveState:       saveState,
		save:            options.Save,
		onDestroy:       options.OnDestroy,
		onCloseCanceled: options.OnCloseCanceled,
		cancelCache:     options.OnCancelCache,
		logger:          options.Logger,
		cacheState:      "",
		saveProgress:    newProgressState(options.ProgressDelay),
		image:           raster,
	}
	doc.native.handler = doc

	hwnd, err := createNativeWindow(
		class,
		0,
		co.WS_POPUP|co.WS_THICKFRAME|co.WS_SYSMENU|co.WS_MINIMIZEBOX|
			co.WS_MAXIMIZEBOX|co.WS_CLIPCHILDREN|co.WS_VISIBLE,
		doc.title(),
		options.Position,
		win.SIZE{
			Cx: int32(scaleDIP(windowWidthDIP, dpiFor(0))),
			Cy: int32(scaleDIP(windowHeightDIP, dpiFor(0))),
		},
		0,
		&doc.native,
	)
	if err != nil {
		return nil, err
	}
	doc.native.hwnd = hwnd

	dpi := dpiFor(hwnd)
	doc.fonts, err = fontsForDPI(dpi)
	if err != nil {
		_ = hwnd.DestroyWindow()
		return nil, err
	}
	if err := doc.createEditor(); err != nil {
		_ = hwnd.DestroyWindow()
		return nil, err
	}
	documentRegistry.Lock()
	documentRegistry.documents[hwnd] = doc
	documentRegistry.Unlock()
	return doc, nil
}

func (d *Document) Hwnd() win.HWND {
	return d.native.hwnd
}

func (d *Document) Snapshot() core.Snapshot {
	return d.snapshot
}

func (d *Document) SaveState() *core.SaveState {
	return d.saveState
}

func (d *Document) Show() {
	if d.native.hwnd == 0 {
		return
	}
	d.native.hwnd.ShowWindow(co.SW_SHOW)
	d.native.hwnd.UpdateWindow()
	if d.edit != 0 {
		_, _ = d.edit.SetFocus()
	}
}

func (d *Document) Close() {
	_ = d.requestClose()
}

func (d *Document) requestClose() documentCloseDisposition {
	if d.native.hwnd == 0 || d.destroyed {
		return documentCloseDestroyed
	}
	if d.saveInProgress && d.saveState != nil && d.saveState.Dirty() {
		// A tray exit may arrive while Ctrl+S is already running. Retain both
		// intents so the window closes after that save reaches a terminal state.
		d.closeAfterSave = true
	}
	_, _ = d.native.hwnd.SendMessage(co.WM_CLOSE, 0, 0)
	if d.destroyed {
		return documentCloseDestroyed
	}
	if d.closeAfterSave && d.saveInProgress {
		return documentCloseDeferred
	}
	return documentCloseCanceled
}

func (d *Document) SetCacheStatus(status string) {
	d.cacheState = status
	d.cacheCancelable = false
	if d.native.hwnd != 0 {
		_ = d.native.hwnd.InvalidateRect(nil, true)
	}
}

func (d *Document) SetCacheProgress(status string) {
	d.cacheState = status
	d.cacheCancelable = true
	if d.native.hwnd != 0 {
		_ = d.native.hwnd.InvalidateRect(nil, true)
	}
}

func (d *Document) createEditor() error {
	if d.snapshot.Kind != core.KindText {
		return nil
	}
	dpi := dpiFor(d.native.hwnd)
	editStyle := co.WS_CHILD | co.WS_VISIBLE | co.WS_VSCROLL | co.WS_HSCROLL |
		co.WS(co.ES_MULTILINE|co.ES_AUTOVSCROLL|co.ES_AUTOHSCROLL|
			co.ES_WANTRETURN|co.ES_NOHIDESEL)
	edit, err := win.CreateWindowEx(
		0,
		win.ClassNameStr("EDIT"),
		"",
		editStyle,
		win.POINT{},
		win.SIZE{},
		d.native.hwnd,
		win.HMENU(editControlID),
		win.HINSTANCE(0),
		win.LPARAM(0),
	)
	if err != nil {
		return fmt.Errorf("create document editor: %w", err)
	}
	d.edit = edit
	if err := edit.SetWindowText(d.snapshot.Text); err != nil {
		return fmt.Errorf("set document editor text: %w", err)
	}
	if _, err := edit.SendMessage(co.WM_SETFONT, win.WPARAM(d.fonts.ui), 1); err != nil {
		return fmt.Errorf("set document editor font: %w", err)
	}
	d.layoutEditor()
	_ = d.native.hwnd.InvalidateRect(nil, true)
	_ = d.native.hwnd.UpdateWindow()
	_ = dpi
	return nil
}

func (d *Document) handleNativeMessage(
	msg co.WM,
	wParam win.WPARAM,
	lParam win.LPARAM,
) (uintptr, bool) {
	switch msg {
	case co.WM_NCCALCSIZE:
		if wParam != 0 {
			return 0, true
		}
	case co.WM_GETMINMAXINFO:
		d.handleMinMaxInfo(lParam)
		return 0, true
	case co.WM_NCHITTEST:
		dpi := dpiFor(d.native.hwnd)
		screenX, screenY := decodeMousePosition(uintptr(lParam))
		point := win.POINT{X: int32(screenX), Y: int32(screenY)}
		if err := d.native.hwnd.ScreenToClientPt(&point); err != nil {
			return 0, false
		}
		width, height, err := clientSize(d.native.hwnd)
		if err != nil {
			return 0, false
		}
		return uintptr(titlebarHitTest(width, height, dpi, int(point.X), int(point.Y))), true
	case co.WM_DPICHANGED:
		d.handleDPIChanged(lParam)
		return 0, true
	case co.WM_ERASEBKGND:
		return 1, true
	case co.WM_PAINT:
		return d.handlePaint()
	case co.WM_SIZE:
		d.layoutEditor()
		_ = d.native.hwnd.InvalidateRect(nil, true)
		return 0, false
	case co.WM_CTLCOLOREDIT:
		brush, err := brushFor(colorWindowBackground)
		if err != nil {
			return 0, false
		}
		hdc := win.HDC(wParam)
		_, _ = hdc.SetTextColor(colorTextPrimary)
		_, _ = hdc.SetBkColor(colorWindowBackground)
		return uintptr(brush), true
	case co.WM_TIMER:
		if int(wParam) != saveProgressTimerID {
			return 0, false
		}
		d.handleSaveTimer()
		return 0, true
	case saveCompleteMessage:
		d.handleSaveCompletion()
		return 0, true
	case co.WM_COMMAND:
		d.handleCommand(wParam)
		return 0, true
	case co.WM_LBUTTONUP:
		d.handleClick(lParam)
		return 0, true
	case co.WM_MOUSEMOVE:
		d.handleMouseMove(lParam)
		return 0, true
	case co.WM_MOUSELEAVE:
		d.hover = titlebarButtonNone
		d.tracking = false
		_ = d.native.hwnd.InvalidateRect(nil, true)
		return 0, true
	case co.WM_CLOSE:
		if !d.confirmClose() {
			return 0, true
		}
		if err := d.native.hwnd.DestroyWindow(); err != nil {
			d.logError("destroy document window", err)
			_, _ = d.native.hwnd.MessageBox(err.Error(), "关闭窗口失败", co.MB_OK|co.MB_ICONERROR)
		}
		return 0, true
	case co.WM_NCDESTROY:
		d.handleDestroyed()
	}
	return 0, false
}

func (d *Document) handleMinMaxInfo(lParam win.LPARAM) {
	info := (*win.MINMAXINFO)(pointerFromUintptr(uintptr(lParam)))
	dpi := dpiFor(d.native.hwnd)
	info.PtMinTrackSize = win.POINT{
		X: int32(scaleDIP(minWindowWidthDIP, dpi)),
		Y: int32(scaleDIP(minWindowHeightDIP, dpi)),
	}
}

func (d *Document) handleDPIChanged(lParam win.LPARAM) {
	suggested := (*win.RECT)(pointerFromUintptr(uintptr(lParam)))
	if suggested == nil {
		return
	}
	_ = d.native.hwnd.MoveWindow(
		win.POINT{X: suggested.Left, Y: suggested.Top},
		win.SIZE{
			Cx: suggested.Right - suggested.Left,
			Cy: suggested.Bottom - suggested.Top,
		},
		true,
	)
	d.layoutEditor()
	_ = d.native.hwnd.InvalidateRect(nil, true)
}

func (d *Document) handlePaint() (uintptr, bool) {
	var paint win.PAINTSTRUCT
	hdc, err := d.native.hwnd.BeginPaint(&paint)
	if err != nil {
		return 0, false
	}
	defer d.native.hwnd.EndPaint(&paint)
	if err := d.paint(hdc); err != nil {
		d.logError("paint document window", err)
		return 0, false
	}
	return 0, true
}

func (d *Document) paint(hdc win.HDC) error {
	width, height, err := clientSize(d.native.hwnd)
	if err != nil {
		return err
	}
	dpi := dpiFor(d.native.hwnd)
	titleHeight := scaleDIP(titlebarHeightDIP, dpi)
	statusHeight := scaleDIP(statusbarHeightDIP, dpi)

	background, err := brushFor(colorWindowBackground)
	if err != nil {
		return err
	}
	if err := hdc.FillRect(&win.RECT{
		Left: 0, Top: 0, Right: int32(width), Bottom: int32(height),
	}, background); err != nil {
		return fmt.Errorf("paint document background: %w", err)
	}

	titlebar, err := brushFor(colorTitlebar)
	if err != nil {
		return err
	}
	if err := hdc.FillRect(&win.RECT{
		Left: 0, Top: 0, Right: int32(width), Bottom: int32(titleHeight),
	}, titlebar); err != nil {
		return fmt.Errorf("paint titlebar: %w", err)
	}
	if err := d.paintButtonBackground(hdc, width, dpi); err != nil {
		return err
	}
	if err := d.paintTitle(hdc, width, titleHeight, dpi); err != nil {
		return err
	}

	border, err := brushFor(colorTitlebarBorder)
	if err != nil {
		return err
	}
	if err := hdc.FillRect(&win.RECT{
		Left:   0,
		Top:    int32(titleHeight - scaleDIP(onePixelDIP, dpi)),
		Right:  int32(width),
		Bottom: int32(titleHeight),
	}, border); err != nil {
		return fmt.Errorf("paint titlebar border: %w", err)
	}
	if err := d.paintImageContent(hdc, width, height, titleHeight, scaleDIP(statusbarHeightDIP, dpi), dpi); err != nil {
		return err
	}

	statusTop := height - statusHeight
	statusbar, err := brushFor(colorStatusbar)
	if err != nil {
		return err
	}
	if err := hdc.FillRect(&win.RECT{
		Left: 0, Top: int32(statusTop), Right: int32(width), Bottom: int32(height),
	}, statusbar); err != nil {
		return fmt.Errorf("paint status bar: %w", err)
	}
	if err := hdc.FillRect(&win.RECT{
		Left:   0,
		Top:    int32(statusTop),
		Right:  int32(width),
		Bottom: int32(statusTop + scaleDIP(onePixelDIP, dpi)),
	}, border); err != nil {
		return fmt.Errorf("paint status bar border: %w", err)
	}
	return d.paintStatus(hdc, width, statusTop, statusHeight, dpi)
}

func (d *Document) paintButtonBackground(hdc win.HDC, width, dpi int) error {
	if d.hover == titlebarButtonNone {
		return nil
	}
	titleHeight := scaleDIP(titlebarHeightDIP, dpi)
	color := colorTitlebarHover
	if d.hover == titlebarButtonClose {
		color = colorCloseHover
	}
	brush, err := brushFor(color)
	if err != nil {
		return err
	}
	rect := win.RECT{Top: 0, Bottom: int32(titleHeight)}
	switch d.hover {
	case titlebarButtonSave:
		rect.Right = int32(scaleDIP(saveButtonWidthDIP, dpi))
	case titlebarButtonClose:
		rect.Left = int32(width - scaleDIP(closeButtonWidthDIP, dpi))
		rect.Right = int32(width)
	}
	if err := hdc.FillRect(&rect, brush); err != nil {
		return fmt.Errorf("paint titlebar button hover: %w", err)
	}
	return nil
}

func (d *Document) paintTitle(hdc win.HDC, width, titleHeight, dpi int) error {
	if _, err := hdc.SetBkMode(co.BKMODE_TRANSPARENT); err != nil {
		return fmt.Errorf("set titlebar text background mode: %w", err)
	}

	saveWidth := scaleDIP(saveButtonWidthDIP, dpi)
	if err := drawGlyphCentered(
		hdc,
		win.RECT{Left: 0, Top: 0, Right: int32(saveWidth), Bottom: int32(titleHeight)},
		"\uE74E",
		d.fonts.icon,
		colorTextPrimary,
	); err != nil {
		return err
	}

	iconLeft := saveWidth + scaleDIP(titleIconGapDIP, dpi)
	iconTop := (titleHeight - scaleDIP(appIconWidthDIP, dpi)) / 2
	if err := d.paintAppIcon(hdc, iconLeft, iconTop, dpi); err != nil {
		return err
	}

	titleLeft := iconLeft + scaleDIP(appIconWidthDIP+titleTextGapDIP, dpi)
	titleRight := width - scaleDIP(closeButtonWidthDIP, dpi)
	title := fitText(
		hdc,
		d.title(),
		titleRight-titleLeft-scaleDIP(titleTextGapDIP, dpi),
		d.fonts.title,
	)
	if err := drawTextVerticallyCentered(
		hdc,
		win.POINT{X: int32(titleLeft), Y: 0},
		win.SIZE{Cx: int32(titleRight - titleLeft), Cy: int32(titleHeight)},
		title,
		d.fonts.title,
		colorTextPrimary,
	); err != nil {
		return err
	}

	closeColor := colorTextPrimary
	if d.hover == titlebarButtonClose {
		closeColor = colorCloseHoverText
	}
	return drawGlyphCentered(
		hdc,
		win.RECT{
			Left:   int32(width - scaleDIP(closeButtonWidthDIP, dpi)),
			Top:    0,
			Right:  int32(width),
			Bottom: int32(titleHeight),
		},
		"\uE8BB",
		d.fonts.icon,
		closeColor,
	)
}

func (d *Document) paintAppIcon(hdc win.HDC, left, top, dpi int) error {
	size := scaleDIP(appIconWidthDIP, dpi)
	stroke := scaleDIP(onePixelDIP, dpi)
	brush, err := brushFor(colorAccent)
	if err != nil {
		return err
	}
	rects := []win.RECT{
		{Left: int32(left + size/4), Top: int32(top), Right: int32(left + 3*size/4), Bottom: int32(top + stroke)},
		{Left: int32(left), Top: int32(top + size/5), Right: int32(left + size), Bottom: int32(top + size)},
		{Left: int32(left + size/4), Top: int32(top + size/2), Right: int32(left + size/2), Bottom: int32(top + size/2 + stroke)},
		{Left: int32(left + size/2), Top: int32(top + size/2), Right: int32(left + 3*size/4), Bottom: int32(top + size/2 + stroke)},
	}
	for _, rect := range rects {
		if err := hdc.FillRect(&rect, brush); err != nil {
			return fmt.Errorf("paint app icon: %w", err)
		}
	}
	return nil
}

func (d *Document) paintStatus(
	hdc win.HDC,
	width, top, height, dpi int,
) error {
	if _, err := hdc.SetBkMode(co.BKMODE_TRANSPARENT); err != nil {
		return fmt.Errorf("set status bar text background mode: %w", err)
	}
	status := d.statusText()
	textRightPadding := scaleDIP(statusTextPaddingDIP, dpi)
	if d.activityCancelable() {
		textRightPadding += scaleDIP(statusCancelWidthDIP, dpi)
	}
	textWidth := width - scaleDIP(statusTextPaddingDIP, dpi) - textRightPadding
	if textWidth < 0 {
		textWidth = 0
	}
	if err := drawTextVerticallyCentered(
		hdc,
		win.POINT{
			X: int32(scaleDIP(statusTextPaddingDIP, dpi)),
			Y: int32(top),
		},
		win.SIZE{
			Cx: int32(textWidth),
			Cy: int32(height),
		},
		fitText(hdc, status, textWidth, d.fonts.ui),
		d.fonts.ui,
		colorTextMuted,
	); err != nil {
		return err
	}
	if !d.activityCancelable() {
		return nil
	}
	rect := statusCancelRect(width, top, height, dpi)
	return drawTextVerticallyCentered(
		hdc,
		win.POINT{
			X: rect.Left,
			Y: int32(top),
		},
		win.SIZE{
			Cx: rect.Right - rect.Left,
			Cy: int32(height),
		},
		"取消",
		d.fonts.ui,
		colorAccent,
	)
}

func drawGlyphCentered(
	hdc win.HDC,
	rect win.RECT,
	text string,
	font win.HFONT,
	color win.COLORREF,
) error {
	oldFont, err := hdc.SelectObjectFont(font)
	if err != nil {
		return fmt.Errorf("select glyph font: %w", err)
	}
	defer func() {
		_, _ = hdc.SelectObjectFont(oldFont)
	}()
	extent, err := hdc.GetTextExtentPoint32(text)
	if err != nil {
		return fmt.Errorf("measure titlebar glyph: %w", err)
	}
	metrics, err := hdc.GetTextMetrics()
	if err != nil {
		return fmt.Errorf("measure titlebar glyph font: %w", err)
	}
	x := int(rect.Left) + (int(rect.Right-rect.Left)-int(extent.Cx))/2
	y := int(rect.Top) + (int(rect.Bottom-rect.Top)-int(metrics.Height))/2
	if _, err := hdc.SetTextColor(color); err != nil {
		return fmt.Errorf("set titlebar glyph color: %w", err)
	}
	if err := hdc.TextOut(x, y, text); err != nil {
		return fmt.Errorf("paint titlebar glyph: %w", err)
	}
	return nil
}

func drawTextVerticallyCentered(
	hdc win.HDC,
	origin win.POINT,
	size win.SIZE,
	text string,
	font win.HFONT,
	color win.COLORREF,
) error {
	oldFont, err := hdc.SelectObjectFont(font)
	if err != nil {
		return fmt.Errorf("select text font: %w", err)
	}
	defer func() {
		_, _ = hdc.SelectObjectFont(oldFont)
	}()
	metrics, err := hdc.GetTextMetrics()
	if err != nil {
		return fmt.Errorf("measure text font: %w", err)
	}
	y := int(origin.Y) + (int(size.Cy)-int(metrics.Height))/2
	if _, err := hdc.SetTextColor(color); err != nil {
		return fmt.Errorf("set text color: %w", err)
	}
	if err := hdc.TextOut(int(origin.X), y, text); err != nil {
		return fmt.Errorf("paint text: %w", err)
	}
	return nil
}

func fitText(hdc win.HDC, text string, maxWidth int, font win.HFONT) string {
	oldFont, err := hdc.SelectObjectFont(font)
	if err != nil {
		return text
	}
	defer func() {
		_, _ = hdc.SelectObjectFont(oldFont)
	}()
	if size, err := hdc.GetTextExtentPoint32(text); err == nil && int(size.Cx) <= maxWidth {
		return text
	}

	const ellipsis = "\u2026"
	runes := []rune(text)
	for count := len(runes) - 1; count >= 0; count-- {
		candidate := string(runes[:count]) + ellipsis
		size, err := hdc.GetTextExtentPoint32(candidate)
		if err == nil && int(size.Cx) <= maxWidth {
			return candidate
		}
	}
	return ellipsis
}

func (d *Document) layoutEditor() {
	if d.edit == 0 || d.native.hwnd == 0 {
		return
	}
	width, height, err := clientSize(d.native.hwnd)
	if err != nil {
		return
	}
	dpi := dpiFor(d.native.hwnd)
	padding := scaleDIP(contentPaddingDIP, dpi)
	top := scaleDIP(titlebarHeightDIP, dpi)
	bottom := scaleDIP(statusbarHeightDIP, dpi)
	editHeight := height - top - bottom
	editWidth := width - 2*padding
	if editHeight < 0 {
		editHeight = 0
	}
	if editWidth < 0 {
		editWidth = 0
	}
	_ = d.edit.MoveWindow(
		win.POINT{X: int32(padding), Y: int32(top)},
		win.SIZE{Cx: int32(editWidth), Cy: int32(editHeight)},
		true,
	)
}

func (d *Document) handleCommand(wParam win.WPARAM) {
	controlID := uint16(wParam)
	notification := uint16(wParam >> 16)
	if controlID != editControlID || notification != uint16(co.EN_CHANGE) {
		return
	}
	text, err := d.edit.GetWindowText()
	if err != nil {
		d.logError("read edited document text", err)
		return
	}
	d.saveState.SetText(text)
	d.refreshTitle()
	_ = d.native.hwnd.InvalidateRect(nil, true)
}

func (d *Document) handleClick(lParam win.LPARAM) {
	width, height, err := clientSize(d.native.hwnd)
	if err != nil {
		d.logError("measure document window for click", err)
		return
	}
	x, y := decodeMousePosition(uintptr(lParam))
	dpi := dpiFor(d.native.hwnd)
	if d.activityCancelable() && statusCancelHit(width, height, dpi, x, y) {
		if d.saveCancelable {
			d.saveCancelable = false
			d.saveStatus = "正在取消保存…"
			if d.saveCancel != nil {
				d.saveCancel()
			}
			_ = d.native.hwnd.InvalidateRect(nil, true)
			return
		}
		d.cacheCancelable = false
		d.cacheState = "正在取消缓存…"
		if d.cancelCache != nil {
			d.cancelCache()
		}
		_ = d.native.hwnd.InvalidateRect(nil, true)
		return
	}
	switch titlebarButtonAt(width, height, dpi, x, y) {
	case titlebarButtonSave:
		shift, _ := win.GetKeyState(co.VK_SHIFT)
		d.saveDocument(shift)
	case titlebarButtonClose:
		_ = d.native.hwnd.PostMessage(co.WM_CLOSE, 0, 0)
	}
}

func (d *Document) handleMouseMove(lParam win.LPARAM) {
	width, height, err := clientSize(d.native.hwnd)
	if err != nil {
		d.logError("measure document window for mouse move", err)
		return
	}
	x, y := decodeMousePosition(uintptr(lParam))
	next := titlebarButtonAt(width, height, dpiFor(d.native.hwnd), x, y)
	if next != d.hover {
		d.hover = next
		_ = d.native.hwnd.InvalidateRect(&win.RECT{
			Left: 0, Top: 0, Right: int32(width),
			Bottom: int32(scaleDIP(titlebarHeightDIP, dpiFor(d.native.hwnd))),
		}, true)
	}
	if !d.tracking {
		track := win.TRACKMOUSEEVENT{
			DwFlags:   co.TME_LEAVE,
			HwndTrack: d.native.hwnd,
		}
		track.SetCbSize()
		if err := win.TrackMouseEvent(&track); err == nil {
			d.tracking = true
		}
	}
}

func (d *Document) handleDestroyed() {
	if d.destroyed {
		return
	}
	d.destroyed = true
	_ = d.native.hwnd.KillTimer(saveProgressTimerID)
	if d.saveCancel != nil {
		d.saveCancel()
		d.saveCancel = nil
	}
	documentRegistry.Lock()
	delete(documentRegistry.documents, d.native.hwnd)
	documentRegistry.Unlock()
	if d.onDestroy != nil {
		d.onDestroy(d)
	}
}

func (d *Document) saveDocument(saveAs bool) bool {
	if d.saveInProgress {
		d.saveStatus = "正在保存，请稍候"
		_ = d.native.hwnd.InvalidateRect(nil, true)
		return false
	}
	if d.save == nil {
		err := errors.New("document save service is not configured")
		d.logError("save document", err)
		_, _ = d.native.hwnd.MessageBox(
			err.Error(),
			"无法保存",
			co.MB_OK|co.MB_ICONERROR,
		)
		return false
	}
	job, err := d.save(d.native.hwnd, saveAs)
	if err != nil {
		if errors.Is(err, save.ErrCanceled) {
			return false
		}
		d.logError("save document", err)
		_, _ = d.native.hwnd.MessageBox(
			fmt.Sprintf("%v\n\n文件未被修改。", err),
			"保存失败",
			co.MB_OK|co.MB_ICONERROR,
		)
		return false
	}
	if job.Run == nil {
		err := errors.New("document save job has no run function")
		d.logError("save document", err)
		return false
	}

	ctx, cancel := context.WithCancel(context.Background())
	d.saveCancel = cancel
	d.saveInProgress = true
	d.saveStatus = ""
	d.saveCancelable = false
	d.saveStarted = time.Now()
	delay := d.saveProgress.delay
	d.saveProgress = newProgressState(delay)
	d.saveProgress.Start(d.saveStarted)

	delayMS := int(delay / time.Millisecond)
	if delayMS < 1 {
		delayMS = 1
	}
	if err := d.native.hwnd.SetTimer(saveProgressTimerID, delayMS); err != nil {
		d.logError("schedule save progress", err)
		d.saveProgress.Finish()
		d.saveStatus = "正在保存…"
		d.saveCancelable = true
	}
	_ = d.native.hwnd.InvalidateRect(nil, true)
	go d.runSaveJob(ctx, job)
	return true
}

func (d *Document) runSaveJob(ctx context.Context, job DocumentSaveJob) {
	result, err := job.Run(ctx)
	d.saveResultMu.Lock()
	d.saveResult = &saveCompletion{result: result, err: err}
	d.saveResultMu.Unlock()
	if postErr := d.native.hwnd.PostMessage(saveCompleteMessage, 0, 0); postErr != nil {
		if d.logger != nil {
			d.logger.Error(
				"post document-save completion",
				"capture_id", d.snapshot.CaptureID,
				"error", postErr,
			)
		}
	}
}

func (d *Document) handleSaveTimer() {
	if !d.saveInProgress {
		_ = d.native.hwnd.KillTimer(saveProgressTimerID)
		return
	}
	_ = d.native.hwnd.KillTimer(saveProgressTimerID)
	if !d.saveProgress.Visible(time.Now()) {
		remaining := d.saveProgress.delay - time.Since(d.saveProgress.started)
		if remaining < time.Millisecond {
			remaining = time.Millisecond
		}
		if err := d.native.hwnd.SetTimer(
			saveProgressTimerID,
			int(remaining/time.Millisecond),
		); err != nil {
			d.logError("reschedule save progress", err)
			d.saveProgress.Finish()
			d.saveStatus = "正在保存…"
			d.saveCancelable = true
			_ = d.native.hwnd.InvalidateRect(nil, true)
		}
		return
	}
	d.saveProgress.Finish()
	d.saveStatus = "正在保存…"
	d.saveCancelable = true
	_ = d.native.hwnd.InvalidateRect(nil, true)
}

func (d *Document) handleSaveCompletion() {
	d.saveResultMu.Lock()
	completion := d.saveResult
	d.saveResult = nil
	d.saveResultMu.Unlock()
	if completion == nil {
		return
	}

	_ = d.native.hwnd.KillTimer(saveProgressTimerID)
	if d.saveCancel != nil {
		d.saveCancel()
		d.saveCancel = nil
	}
	d.saveProgress.Finish()
	d.saveInProgress = false
	d.saveCancelable = false
	closeAfterSave := d.closeAfterSave
	d.closeAfterSave = false

	switch {
	case completion.err != nil &&
		(errors.Is(completion.err, save.ErrCanceled) ||
			errors.Is(completion.err, context.Canceled)):
		d.saveStatus = "保存已取消"
		d.closeAfterSave = false
	case completion.err != nil:
		d.saveStatus = "保存失败"
		d.closeAfterSave = false
		d.logError("save document", completion.err)
		_, _ = d.native.hwnd.MessageBox(
			fmt.Sprintf("%v\n\n文件未被修改。", completion.err),
			"保存失败",
			co.MB_OK|co.MB_ICONERROR,
		)
	default:
		d.saveState.MarkSavedText(completion.result.Path, completion.result.Text)
		d.saveStatus = "已保存"
		if d.logger != nil {
			d.logger.Info(
				"document saved",
				"capture_id", d.snapshot.CaptureID,
				"kind", d.snapshot.Kind,
				"path", completion.result.Path,
				"duration", time.Since(d.saveStarted),
			)
		}
	}

	d.refreshTitle()
	_ = d.native.hwnd.InvalidateRect(nil, true)
	if completion.err != nil && closeAfterSave {
		d.notifyCloseCanceled()
	}
	if completion.err == nil && closeAfterSave {
		_ = d.native.hwnd.PostMessage(co.WM_CLOSE, 0, 0)
	}
}

func (d *Document) confirmClose() bool {
	if d.saveState == nil || !d.saveState.Dirty() {
		return true
	}
	if d.saveInProgress {
		d.saveStatus = "正在保存，请稍候"
		_ = d.native.hwnd.InvalidateRect(nil, true)
		return false
	}
	answer, err := d.native.hwnd.MessageBox(
		"这篇剪贴板文本有未保存的修改。\n\n是否在关闭前保存？",
		"未保存的修改",
		co.MB_YESNOCANCEL|co.MB_ICONWARNING|co.MB_DEFBUTTON1,
	)
	if err != nil {
		d.logError("show unsaved-changes prompt", err)
		d.notifyCloseCanceled()
		return false
	}
	switch answer {
	case co.ID_YES:
		d.closeAfterSave = true
		if !d.saveDocument(false) {
			d.closeAfterSave = false
			d.notifyCloseCanceled()
		}
		return false
	case co.ID_NO:
		return true
	default:
		d.notifyCloseCanceled()
		return false
	}
}

func (d *Document) notifyCloseCanceled() {
	if d.onCloseCanceled != nil {
		d.onCloseCanceled(d)
	}
}

func (d *Document) executeCommand(command Command) {
	switch command {
	case CommandSave:
		d.saveDocument(false)
	case CommandSaveAs:
		d.saveDocument(true)
	case CommandClose:
		_ = d.native.hwnd.PostMessage(co.WM_CLOSE, 0, 0)
	}
}

func (d *Document) logError(message string, err error) {
	if d.logger != nil {
		d.logger.Error(
			message,
			"capture_id", d.snapshot.CaptureID,
			"kind", d.snapshot.Kind,
			"error", err,
		)
	}
}

func (d *Document) refreshTitle() {
	title := d.title()
	_ = d.native.hwnd.SetWindowText(title)
	_ = d.native.hwnd.InvalidateRect(nil, true)
}

func (d *Document) title() string {
	title := "ClipNote - 剪贴板文本"
	if d.snapshot.Kind == core.KindImage {
		title = "ClipNote - 剪贴板图片"
	}
	if d.saveState != nil && d.saveState.Dirty() {
		title += " *"
	}
	return title
}

func (d *Document) statusText() string {
	var content string
	switch d.snapshot.Kind {
	case core.KindImage:
		content = fmt.Sprintf(
			"图片 · %d × %d · %s",
			d.image.Width,
			d.image.Height,
			formatByteSize(int64(len(d.image.Pixels))),
		)
	default:
		content = fmt.Sprintf(
			"文本 · %d 字符",
			utf8.RuneCountInString(d.saveState.Text()),
		)
	}
	if d.cacheState != "" {
		content += " · " + d.cacheState
	}
	if d.saveStatus != "" {
		content += " · " + d.saveStatus
	}
	if d.saveState.Path() != "" {
		content += " · " + d.saveState.Path()
	}
	return content
}

func (d *Document) activityCancelable() bool {
	return d.saveCancelable || (!d.saveInProgress && d.cacheCancelable)
}

func formatByteSize(bytes int64) string {
	const (
		kib = 1024
		mib = 1024 * kib
	)
	switch {
	case bytes >= mib:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/mib)
	case bytes >= kib:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/kib)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func statusCancelRect(width, top, height, dpi int) win.RECT {
	right := width - scaleDIP(statusTextPaddingDIP, dpi)
	left := right - scaleDIP(statusCancelWidthDIP, dpi)
	return win.RECT{
		Left:   int32(left),
		Top:    int32(top),
		Right:  int32(right),
		Bottom: int32(top + height),
	}
}

func statusCancelHit(width, height, dpi, x, y int) bool {
	statusHeight := scaleDIP(statusbarHeightDIP, dpi)
	rect := statusCancelRect(width, height-statusHeight, statusHeight, dpi)
	return x >= int(rect.Left) && x < int(rect.Right) &&
		y >= int(rect.Top) && y < int(rect.Bottom)
}

func dispatchDocumentShortcut(msg *win.MSG) bool {
	if msg == nil || msg.Msg != co.WM_KEYDOWN {
		return false
	}
	ctrl, _ := win.GetKeyState(co.VK_CONTROL)
	if !ctrl {
		return false
	}
	root, err := msg.HWnd.GetAncestor(co.GA_ROOT)
	if err != nil || root == 0 {
		return false
	}
	documentRegistry.RLock()
	doc := documentRegistry.documents[root]
	documentRegistry.RUnlock()
	if doc == nil {
		return false
	}
	shift, _ := win.GetKeyState(co.VK_SHIFT)
	command := CommandFor(co.VK(uintptr(msg.WParam)), shift)
	if command == CommandNone {
		return false
	}
	doc.executeCommand(command)
	return true
}

func createDocumentForTest(snapshot core.Snapshot) (*Document, error) {
	return NewDocument(DocumentOptions{Snapshot: snapshot})
}

func validateDocumentSnapshot(snapshot core.Snapshot) error {
	if snapshot.CaptureID == "" {
		return errors.New("create document: capture ID is empty")
	}
	if snapshot.Kind == 0 {
		return errors.New("create document: content kind is unset")
	}
	return nil
}
