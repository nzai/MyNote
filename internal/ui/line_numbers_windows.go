//go:build windows

package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const (
	lineNumberGutterMinDIP     = 44
	lineNumberGutterPaddingDIP = 8
	lineNumberDigitWidthDIP    = 8
	lineNumberEditSubclassID   = 1

	wmMouseWheel co.WM = 0x020a
)

var editorLineEndings = strings.NewReplacer(
	"\r\n", "\r\n",
	"\r", "\r\n",
	"\n", "\r\n",
)

type lineNumberRow struct {
	number int
	y      int
}

var lineNumberEditSubclass = struct {
	sync.Once
	proc uintptr
}{}

// LF-only text makes the native EDIT control assign several logical lines to
// the same pixel row, which also breaks hit-testing and selection.
func normalizeEditorText(text string) string {
	return editorLineEndings.Replace(text)
}

// logicalLineStarts returns character offsets in the UTF-16 stream stored by
// the native EDIT control after line endings have been normalized to CRLF.
func logicalLineStarts(text string) []int {
	starts, _ := editorTextLayout(text)
	return starts
}

func editorTextLayout(text string) ([]int, int) {
	text = normalizeEditorText(text)
	starts := []int{0}
	offset := 0
	for _, r := range text {
		offset += utf16.RuneLen(r)
		if r == '\n' {
			starts = append(starts, offset)
		}
	}
	return starts, offset
}

func lineNumberGutterWidthDIP(lastLineNumber int) int {
	if lastLineNumber < 1 {
		lastLineNumber = 1
	}
	digits := len(strconv.Itoa(lastLineNumber))
	width := 2*lineNumberGutterPaddingDIP + digits*lineNumberDigitWidthDIP
	if width < lineNumberGutterMinDIP {
		width = lineNumberGutterMinDIP
	}
	return width
}

func (d *Document) lineNumberGutterWidth(dpi int) int {
	if !d.lineNumbersVisible() {
		return 0
	}
	return scaleDIP(lineNumberGutterWidthDIP(len(d.lineStarts)), dpi)
}

func (d *Document) visibleLineNumberRows() ([]lineNumberRow, error) {
	if !d.lineNumbersVisible() || len(d.lineStarts) == 0 {
		return nil, nil
	}
	client, err := d.edit.GetClientRect()
	if err != nil {
		return nil, fmt.Errorf("measure editor for line numbers: %w", err)
	}
	firstVisible, err := d.edit.SendMessage(co.EM_GETFIRSTVISIBLELINE, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("query first visible editor line: %w", err)
	}
	firstChar, err := d.edit.SendMessage(
		co.EM_LINEINDEX,
		win.WPARAM(firstVisible),
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("query first visible editor character: %w", err)
	}
	firstIndex := 0
	if int64(firstChar) > 0 {
		firstIndex = int(firstChar)
	}
	start := sort.SearchInts(d.lineStarts, firstIndex)
	lineHeight := d.fontLineHeight()
	if lineHeight < 1 {
		lineHeight = 1
	}
	rows := make([]lineNumberRow, 0, int(client.Bottom)/lineHeight+2)
	for index := start; index < len(d.lineStarts); index++ {
		y, err := d.lineNumberY(d.lineStarts[index])
		if err != nil {
			return nil, err
		}
		if y >= int(client.Bottom) {
			break
		}
		if y+lineHeight > 0 {
			rows = append(rows, lineNumberRow{number: index + 1, y: y})
		}
	}
	return rows, nil
}

func (d *Document) lineNumberY(charIndex int) (int, error) {
	index := sort.SearchInts(d.lineStarts, charIndex)
	if index < len(d.lineStarts) && d.lineStarts[index] == charIndex &&
		d.blankLogicalLine(index) {
		return d.blankLineY(index)
	}
	return d.rawLineNumberY(charIndex)
}

func (d *Document) rawLineNumberY(charIndex int) (int, error) {
	position, err := d.edit.SendMessage(
		co.EM_POSFROMCHAR,
		win.WPARAM(charIndex),
		0,
	)
	if err != nil {
		return 0, fmt.Errorf("position logical line at character %d: %w", charIndex, err)
	}
	x, y := decodeMousePosition(position)
	if x == -1 && y == -1 {
		if charIndex > 0 && charIndex == d.lineStarts[len(d.lineStarts)-1] {
			previousY, err := d.lineNumberY(charIndex - 1)
			if err == nil {
				lineHeight := d.fontLineHeight()
				if lineHeight < 1 {
					lineHeight = 1
				}
				return previousY + lineHeight, nil
			}
		}
		return 0, fmt.Errorf("position logical line at character %d: edit returned no position", charIndex)
	}
	return y, nil
}

func (d *Document) blankLogicalLine(index int) bool {
	if index < 0 || index >= len(d.lineStarts) {
		return false
	}
	start := d.lineStarts[index]
	if index == len(d.lineStarts)-1 {
		return start >= d.editorTextLength
	}
	// A blank logical line contains only the normalized CRLF delimiter.
	return d.lineStarts[index+1]-start <= 2
}

func (d *Document) blankLineY(index int) (int, error) {
	lineHeight := d.fontLineHeight()
	if lineHeight < 1 {
		lineHeight = 1
	}
	next := index + 1
	for next < len(d.lineStarts) && d.blankLogicalLine(next) {
		next++
	}
	if next < len(d.lineStarts) {
		y, err := d.rawLineNumberY(d.lineStarts[next])
		if err != nil {
			return 0, err
		}
		return y - (next-index)*lineHeight, nil
	}

	previous := index - 1
	for previous >= 0 && d.blankLogicalLine(previous) {
		previous--
	}
	if previous < 0 {
		return index * lineHeight, nil
	}
	contentEnd := d.lineStarts[previous+1] - 3
	if contentEnd < d.lineStarts[previous] {
		contentEnd = d.lineStarts[previous]
	}
	y, err := d.rawLineNumberY(contentEnd)
	if err != nil {
		return 0, err
	}
	return y + (index-previous)*lineHeight, nil
}

func (d *Document) fontLineHeight() int {
	if d.edit == 0 || d.editorFont == 0 {
		return 0
	}
	hdc, err := d.edit.GetDC()
	if err != nil {
		return 0
	}
	defer func() {
		_ = d.edit.ReleaseDC(hdc)
	}()
	oldFont, err := hdc.SelectObjectFont(d.editorFont)
	if err != nil {
		return 0
	}
	defer func() {
		_, _ = hdc.SelectObjectFont(oldFont)
	}()
	metrics, err := hdc.GetTextMetrics()
	if err != nil {
		return 0
	}
	height := int(metrics.Height)
	if d.editorSettings.LineSpacing > 100 {
		height = height * d.editorSettings.LineSpacing / 100
	}
	return height
}

func (d *Document) lineNumberGutterRect(
	width, height, titleHeight, statusHeight, dpi int,
) win.RECT {
	padding := scaleDIP(contentPaddingDIP, dpi)
	return win.RECT{
		// Extend through the content inset so the gutter reaches the window
		// edge; the edit control still starts after the inset.
		Left:   0,
		Top:    int32(titleHeight),
		Right:  int32(padding + d.lineNumberGutterWidth(dpi)),
		Bottom: int32(height - statusHeight),
	}
}

func (d *Document) invalidateLineNumberGutter() {
	if !d.lineNumbersVisible() || d.native.hwnd == 0 {
		return
	}
	width, height, err := clientSize(d.native.hwnd)
	if err != nil {
		return
	}
	dpi := dpiFor(d.native.hwnd)
	rect := d.lineNumberGutterRect(
		width,
		height,
		scaleDIP(titlebarHeightDIP, dpi),
		scaleDIP(statusbarHeightDIP, dpi),
		dpi,
	)
	_ = d.native.hwnd.InvalidateRect(&rect, false)
}

func (d *Document) repaintLineNumberGutter() {
	if !d.lineNumbersVisible() || d.native.hwnd == 0 {
		return
	}
	width, height, err := clientSize(d.native.hwnd)
	if err != nil {
		d.logError("measure window for line-number repaint", err)
		return
	}
	dpi := dpiFor(d.native.hwnd)
	rect := d.lineNumberGutterRect(
		width,
		height,
		scaleDIP(titlebarHeightDIP, dpi),
		scaleDIP(statusbarHeightDIP, dpi),
		dpi,
	)
	// The gutter lives on the parent while the native edit scrolls itself.
	// Painting synchronously prevents the gutter from lagging behind the text.
	if err := d.native.hwnd.RedrawWindow(
		&rect,
		0,
		co.RDW_INVALIDATE|co.RDW_UPDATENOW,
	); err != nil {
		d.logError("repaint line-number gutter", err)
	}
}

func (d *Document) installLineNumberRepaintHook() error {
	if !d.lineNumbersVisible() {
		return nil
	}
	win.InitCommonControls()
	lineNumberEditSubclass.Do(func() {
		lineNumberEditSubclass.proc = syscall.NewCallback(
			lineNumberEditSubclassWindowProc,
		)
	})
	// EN_VSCROLL is not emitted for every viewport change, including the
	// repeated WM_VSCROLL messages produced while dragging the scrollbar.
	if err := d.edit.SetWindowSubclass(
		lineNumberEditSubclass.proc,
		lineNumberEditSubclassID,
		nil,
	); err != nil {
		return fmt.Errorf("install line-number scroll repaint hook: %w", err)
	}
	return nil
}

func lineNumberEditSubclassWindowProc(
	hwnd win.HWND,
	msg co.WM,
	wParam win.WPARAM,
	lParam win.LPARAM,
	subclassID uintptr,
	_ uintptr,
) uintptr {
	if msg == co.WM_NCDESTROY {
		_ = hwnd.RemoveWindowSubclass(
			lineNumberEditSubclass.proc,
			uint32(subclassID),
		)
	}
	result := hwnd.DefSubclassProc(msg, wParam, lParam)
	if editorMessageMayMoveViewport(msg) {
		if document := documentForEditor(hwnd); document != nil {
			document.syncLineNumberGutterWithViewport()
		}
	}
	return result
}

func editorMessageMayMoveViewport(msg co.WM) bool {
	switch msg {
	case co.EM_LINESCROLL,
		co.EM_SCROLLCARET,
		co.EM_SETSEL,
		co.WM_VSCROLL,
		co.WM_KEYDOWN,
		co.WM_CHAR,
		co.WM_LBUTTONDOWN,
		co.WM_LBUTTONDBLCLK,
		co.WM_PAINT,
		co.WM_PASTE,
		co.WM_CUT,
		co.WM_CLEAR,
		co.WM_SIZE,
		co.WM_SETTEXT,
		wmMouseWheel:
		return true
	default:
		return false
	}
}

func documentForEditor(editor win.HWND) *Document {
	parent, err := editor.GetParent()
	if err != nil || parent == 0 {
		return nil
	}
	documentRegistry.RLock()
	defer documentRegistry.RUnlock()
	return documentRegistry.documents[parent]
}

func (d *Document) syncLineNumberGutterWithViewport() {
	if !d.lineNumbersVisible() {
		return
	}
	firstVisible, err := d.edit.SendMessage(co.EM_GETFIRSTVISIBLELINE, 0, 0)
	if err != nil {
		d.logError("query first visible line after editor paint", err)
		return
	}
	top := int(firstVisible)
	if top == d.lineNumberFirstVisible {
		return
	}
	d.lineNumberFirstVisible = top
	d.repaintLineNumberGutter()
}

func (d *Document) paintLineNumberGutter(
	hdc win.HDC,
	width, height, titleHeight, statusHeight, dpi int,
) error {
	if !d.lineNumbersVisible() {
		return nil
	}
	rect := d.lineNumberGutterRect(width, height, titleHeight, statusHeight, dpi)
	if rect.Right <= rect.Left || rect.Bottom <= rect.Top {
		return nil
	}
	background, err := brushFor(colorLineGutter)
	if err != nil {
		return err
	}
	if err := hdc.FillRect(&rect, background); err != nil {
		return fmt.Errorf("paint line-number gutter: %w", err)
	}
	border, err := brushFor(colorTitlebarBorder)
	if err != nil {
		return err
	}
	if err := hdc.FillRect(&win.RECT{
		Left:   rect.Right - int32(scaleDIP(onePixelDIP, dpi)),
		Top:    rect.Top,
		Right:  rect.Right,
		Bottom: rect.Bottom,
	}, border); err != nil {
		return fmt.Errorf("paint line-number separator: %w", err)
	}

	rows, err := d.visibleLineNumberRows()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	saved, err := hdc.SaveDC()
	if err != nil {
		return fmt.Errorf("save line-number drawing state: %w", err)
	}
	defer func() {
		_ = hdc.RestoreDC(saved)
	}()
	if _, err := hdc.IntersectClipRect(rect); err != nil {
		return fmt.Errorf("clip line-number gutter: %w", err)
	}
	oldFont, err := hdc.SelectObjectFont(d.fonts.lineNumber)
	if err != nil {
		return fmt.Errorf("select line-number font: %w", err)
	}
	defer func() {
		_, _ = hdc.SelectObjectFont(oldFont)
	}()
	if _, err := hdc.SetBkMode(co.BKMODE_TRANSPARENT); err != nil {
		return fmt.Errorf("set line-number background mode: %w", err)
	}
	if _, err := hdc.SetTextColor(colorTextMuted); err != nil {
		return fmt.Errorf("set line-number color: %w", err)
	}
	metrics, err := hdc.GetTextMetrics()
	if err != nil {
		return fmt.Errorf("measure line-number font: %w", err)
	}
	rightPadding := scaleDIP(lineNumberGutterPaddingDIP, dpi)
	lineHeight := d.fontLineHeight()
	if lineHeight < 1 {
		lineHeight = int(metrics.Height)
	}
	for _, row := range rows {
		label := strconv.Itoa(row.number)
		extent, err := hdc.GetTextExtentPoint32(label)
		if err != nil {
			return fmt.Errorf("measure line number %d: %w", row.number, err)
		}
		x := int(rect.Right) - rightPadding - int(extent.Cx)
		y := titleHeight + row.y + (lineHeight-int(metrics.Height))/2
		if err := hdc.TextOut(x, y, label); err != nil {
			return fmt.Errorf("paint line number %d: %w", row.number, err)
		}
	}
	return nil
}
