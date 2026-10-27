//go:build windows

package ui

import (
	"fmt"
	"strconv"
	"syscall"
	"unsafe"

	"github.com/nzai/MyNote/internal/config"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

var drawOptionsTextWProc = syscall.NewLazyDLL("user32.dll").NewProc("DrawTextW")

func (p *optionsPanel) paint(hdc win.HDC) error {
	return p.paintTo(hdc, hdc)
}

func (p *optionsPanel) paintFrame(target win.HDC) error {
	width, height, err := clientSize(p.native.hwnd)
	if err != nil {
		return err
	}
	return paintDoubleBuffered(target, width, height, func(memory win.HDC) error {
		return p.paintTo(target, memory)
	})
}

func (p *optionsPanel) paintTo(target, hdc win.HDC) error {
	width, height, err := clientSize(p.native.hwnd)
	if err != nil {
		return err
	}
	layout := optionsPanelContentsLayout(
		win.RECT{Right: int32(width), Bottom: int32(height)},
		dpiFor(p.native.hwnd),
	)
	background, err := brushFor(colorOptionsPanel)
	if err != nil {
		return err
	}
	if err := hdc.FillRect(&win.RECT{
		Right:  int32(width),
		Bottom: int32(height),
	}, background); err != nil {
		return fmt.Errorf("paint options panel background: %w", err)
	}
	if p.paintObserver != nil {
		p.paintObserver(target, hdc)
	}
	if err := p.paintDescription(hdc, layout); err != nil {
		return err
	}
	if err := p.paintPreview(hdc, layout); err != nil {
		return err
	}
	if err := p.paintFontSelector(hdc, layout); err != nil {
		return err
	}
	if err := p.paintSliders(hdc, layout); err != nil {
		return err
	}
	if err := p.paintCheckboxes(hdc, layout); err != nil {
		return err
	}
	if err := p.paintFooter(hdc, layout); err != nil {
		return err
	}
	return nil
}

func (p *optionsPanel) paintDescription(hdc win.HDC, layout optionsPanelLayout) error {
	return paintOptionsText(
		hdc,
		layout.description,
		"更改会立即预览；只有点击「保存」才会写入设置。",
		p.document.fonts.ui,
		colorTextMuted,
		co.DT_LEFT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_END_ELLIPSIS|
			co.DT_NOPREFIX,
	)
}

func (p *optionsPanel) paintPreview(hdc win.HDC, layout optionsPanelLayout) error {
	dpi := dpiFor(p.native.hwnd)
	if err := fillOptionsRoundedRect(hdc, layout.preview, 8, colorOptionsPreview); err != nil {
		return err
	}
	if err := strokeOptionsRoundedRect(
		hdc,
		layout.preview,
		8,
		colorOptionsPanelBorder,
	); err != nil {
		return err
	}
	padding := int32(scaleDIP(10, dpi))
	labelRect := win.RECT{
		Left:   layout.preview.Left + padding,
		Top:    layout.preview.Top + int32(scaleDIP(4, dpi)),
		Right:  layout.preview.Right - padding,
		Bottom: layout.preview.Top + int32(scaleDIP(18, dpi)),
	}
	if err := paintOptionsText(
		hdc,
		labelRect,
		"实时预览",
		p.document.fonts.ui,
		colorOptionsAccentStrong,
		co.DT_LEFT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_NOPREFIX,
	); err != nil {
		return err
	}
	metadata := fmt.Sprintf(
		"%s  ·  %d pt  ·  %s",
		p.settings.FontFamily,
		p.settings.FontPointSize,
		lineSpacingDisplay(p.settings.LineSpacing),
	)
	if err := paintOptionsText(
		hdc,
		labelRect,
		metadata,
		p.document.fonts.ui,
		colorTextMuted,
		co.DT_RIGHT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_END_ELLIPSIS|
			co.DT_NOPREFIX,
	); err != nil {
		return err
	}
	previewFont := p.previewFont
	if previewFont == 0 {
		previewFont = p.document.fonts.ui
	}
	textLeft := layout.preview.Left + int32(scaleDIP(12, dpi))
	if p.settings.ShowLineNumbers {
		if err := paintOptionsText(
			hdc,
			win.RECT{
				Left:   layout.preview.Left + int32(scaleDIP(6, dpi)),
				Top:    layout.preview.Top + int32(scaleDIP(20, dpi)),
				Right:  layout.preview.Left + int32(scaleDIP(24, dpi)),
				Bottom: layout.preview.Bottom - int32(scaleDIP(3, dpi)),
			},
			"1",
			p.document.fonts.lineNumber,
			colorTextMuted,
			co.DT_RIGHT|co.DT_TOP|co.DT_SINGLELINE|co.DT_NOPREFIX,
		); err != nil {
			return err
		}
		textLeft = layout.preview.Left + int32(scaleDIP(30, dpi))
	}
	metrics, err := measureTextHeight(hdc, previewFont)
	if err != nil {
		return err
	}
	lineHeight := metrics * p.settings.LineSpacing / 100
	if lineHeight < metrics {
		lineHeight = metrics
	}
	firstTop := int(layout.preview.Top) + scaleDIP(20, dpi)
	lines := []string{
		"ClipNote 剪贴板内容预览",
		"字体、字号与行距会同步到编辑区",
	}
	for index, line := range lines {
		top := firstTop + index*lineHeight
		if top >= int(layout.preview.Bottom) {
			break
		}
		rect := win.RECT{
			Left:   textLeft,
			Top:    int32(top),
			Right:  layout.preview.Right - padding,
			Bottom: int32(min(top+lineHeight, int(layout.preview.Bottom))),
		}
		color := colorTextPrimary
		if index > 0 {
			color = colorTextMuted
		}
		if err := paintOptionsText(
			hdc,
			rect,
			line,
			previewFont,
			color,
			co.DT_LEFT|co.DT_TOP|co.DT_SINGLELINE|co.DT_END_ELLIPSIS|
				co.DT_NOPREFIX,
		); err != nil {
			return err
		}
	}
	return nil
}

func (p *optionsPanel) paintFontSelector(
	hdc win.HDC,
	layout optionsPanelLayout,
) error {
	if err := paintOptionsText(
		hdc,
		layout.fontLabel,
		"字体",
		p.document.fonts.ui,
		colorTextPrimary,
		co.DT_LEFT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_NOPREFIX,
	); err != nil {
		return err
	}
	return strokeOptionsRoundedRect(
		hdc,
		layout.fontSelector,
		5,
		colorOptionsPanelBorder,
	)
}

func (p *optionsPanel) paintSliders(hdc win.HDC, layout optionsPanelLayout) error {
	if err := paintOptionsText(
		hdc,
		layout.sizeLabel,
		"字号",
		p.document.fonts.ui,
		colorTextPrimary,
		co.DT_LEFT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_NOPREFIX,
	); err != nil {
		return err
	}
	if err := paintOptionsText(
		hdc,
		layout.sizeLabel,
		strconv.Itoa(p.settings.FontPointSize)+" pt",
		p.document.fonts.ui,
		colorOptionsAccentStrong,
		co.DT_RIGHT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_NOPREFIX,
	); err != nil {
		return err
	}
	if err := p.paintSlider(
		hdc,
		layout.sizeSlider,
		sliderThumbValue(
			p.settings.FontPointSize,
			config.MinEditorFontPointSize,
			config.MaxEditorFontPointSize,
			layout.sizeSlider,
			dpiFor(p.native.hwnd),
		),
	); err != nil {
		return err
	}

	if err := paintOptionsText(
		hdc,
		layout.spacingLabel,
		"行间距",
		p.document.fonts.ui,
		colorTextPrimary,
		co.DT_LEFT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_NOPREFIX,
	); err != nil {
		return err
	}
	if err := paintOptionsText(
		hdc,
		layout.spacingLabel,
		lineSpacingDisplay(p.settings.LineSpacing),
		p.document.fonts.ui,
		colorOptionsAccentStrong,
		co.DT_RIGHT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_NOPREFIX,
	); err != nil {
		return err
	}
	spacingIndex := editorLineSpacingIndex(p.settings.LineSpacing)
	spacingLeft, spacingRight := sliderTrackRange(
		layout.spacingSlider,
		dpiFor(p.native.hwnd),
	)
	thumbX := sliderThumbX(
		spacingIndex,
		0,
		len(config.LineSpacingOptions())-1,
		spacingLeft,
		spacingRight,
	)
	if err := p.paintSlider(hdc, layout.spacingSlider, thumbX); err != nil {
		return err
	}
	options := config.LineSpacingOptions()
	width := layout.spacingLabels.Right - layout.spacingLabels.Left
	for index, spacing := range options {
		left := layout.spacingLabels.Left +
			int32(index)*width/int32(len(options))
		right := layout.spacingLabels.Left +
			int32(index+1)*width/int32(len(options))
		color := colorTextMuted
		if index == spacingIndex {
			color = colorOptionsAccentStrong
		}
		if err := paintOptionsText(
			hdc,
			win.RECT{
				Left:   left,
				Top:    layout.spacingLabels.Top,
				Right:  right,
				Bottom: layout.spacingLabels.Bottom,
			},
			lineSpacingDisplay(spacing),
			p.document.fonts.ui,
			color,
			co.DT_CENTER|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_NOPREFIX,
		); err != nil {
			return err
		}
	}
	return drawOptionsHorizontalLine(
		hdc,
		int(layout.spacingLabels.Left),
		int(layout.spacingLabels.Right),
		int(layout.spacingLabels.Bottom)+scaleDIP(5, dpiFor(p.native.hwnd)),
		colorOptionsPanelBorder,
	)
}

func (p *optionsPanel) paintSlider(
	hdc win.HDC,
	rect win.RECT,
	thumbX int,
) error {
	dpi := dpiFor(p.native.hwnd)
	left, right := sliderTrackRange(rect, dpi)
	centerY := (rect.Top + rect.Bottom) / 2
	trackHeight := scaleDIP(4, dpi)
	track := win.RECT{
		Left:   int32(left),
		Top:    centerY - int32(trackHeight/2),
		Right:  int32(right),
		Bottom: centerY - int32(trackHeight/2) + int32(trackHeight),
	}
	if err := fillOptionsRoundedRect(hdc, track, 2, colorOptionsTrack); err != nil {
		return err
	}
	progress := track
	progress.Right = int32(thumbX)
	if progress.Right > progress.Left {
		if err := fillOptionsRoundedRect(
			hdc,
			progress,
			2,
			colorOptionsAccent,
		); err != nil {
			return err
		}
	}
	radius := scaleDIP(8, dpi)
	thumb := win.RECT{
		Left:   int32(thumbX - radius),
		Top:    centerY - int32(radius),
		Right:  int32(thumbX + radius),
		Bottom: centerY + int32(radius),
	}
	if err := fillOptionsEllipse(hdc, thumb, colorWindowBackground); err != nil {
		return err
	}
	return strokeOptionsEllipse(hdc, thumb, colorOptionsAccent, max(1, scaleDIP(2, dpi)))
}

func (p *optionsPanel) paintCheckboxes(
	hdc win.HDC,
	layout optionsPanelLayout,
) error {
	items := []struct {
		rect    win.RECT
		label   string
		checked bool
		hit     optionsPanelHit
	}{
		{
			rect:    layout.autoStartCheckbox,
			label:   "随 Windows 启动",
			checked: p.autoStart,
			hit:     optionsHitAutoStart,
		},
		{
			rect:    layout.suppressCloseCheckbox,
			label:   "关闭窗口时不提醒保存",
			checked: p.settings.SuppressClosePrompt,
			hit:     optionsHitSuppressPrompt,
		},
		{
			rect:    layout.lineNumbersCheckbox,
			label:   "显示行号",
			checked: p.settings.ShowLineNumbers,
			hit:     optionsHitLineNumbers,
		},
	}
	for _, item := range items {
		if p.hover == item.hit || p.pressed == item.hit {
			if err := fillOptionsRoundedRect(
				hdc,
				item.rect,
				4,
				colorOptionsAccentSoft,
			); err != nil {
				return err
			}
		}
		if err := p.paintCheckbox(hdc, item.rect, item.checked); err != nil {
			return err
		}
		labelRect := item.rect
		labelRect.Left += int32(scaleDIP(26, dpiFor(p.native.hwnd)))
		if err := paintOptionsText(
			hdc,
			labelRect,
			item.label,
			p.document.fonts.ui,
			colorTextPrimary,
			co.DT_LEFT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_END_ELLIPSIS|
				co.DT_NOPREFIX,
		); err != nil {
			return err
		}
	}
	return nil
}

func (p *optionsPanel) paintCheckbox(
	hdc win.HDC,
	row win.RECT,
	checked bool,
) error {
	dpi := dpiFor(p.native.hwnd)
	size := scaleDIP(optionsCheckboxSizeDIP, dpi)
	top := (row.Top+row.Bottom)/2 - int32(size/2)
	box := win.RECT{
		Left:   row.Left + int32(scaleDIP(2, dpi)),
		Top:    top,
		Right:  row.Left + int32(scaleDIP(2, dpi)+size),
		Bottom: top + int32(size),
	}
	if checked {
		if err := fillOptionsRoundedRect(hdc, box, 4, colorOptionsAccent); err != nil {
			return err
		}
		pen, err := win.CreatePen(co.PS_SOLID, max(1, scaleDIP(2, dpi)), colorWindowBackground)
		if err != nil {
			return err
		}
		defer func() {
			_ = pen.DeleteObject()
		}()
		if err := drawOptionsLine(
			hdc,
			pen,
			int(box.Left)+scaleDIP(4, dpi),
			int(box.Top)+scaleDIP(8, dpi),
			int(box.Left)+scaleDIP(7, dpi),
			int(box.Top)+scaleDIP(12, dpi),
		); err != nil {
			return err
		}
		return drawOptionsLine(
			hdc,
			pen,
			int(box.Left)+scaleDIP(7, dpi),
			int(box.Top)+scaleDIP(12, dpi),
			int(box.Left)+scaleDIP(13, dpi),
			int(box.Top)+scaleDIP(5, dpi),
		)
	}
	if err := fillOptionsRoundedRect(hdc, box, 3, colorWindowBackground); err != nil {
		return err
	}
	return strokeOptionsRoundedRect(hdc, box, 3, colorOptionsPanelBorder)
}

func (p *optionsPanel) paintFooter(hdc win.HDC, layout optionsPanelLayout) error {
	dpi := dpiFor(p.native.hwnd)
	if err := drawOptionsHorizontalLine(
		hdc,
		int(layout.panel.Left)+scaleDIP(optionsPanelPaddingDIP, dpi),
		int(layout.panel.Right)-scaleDIP(optionsPanelPaddingDIP, dpi),
		layout.footerDividerY,
		colorOptionsPanelBorder,
	); err != nil {
		return err
	}
	if p.errorText != "" {
		if err := paintOptionsText(
			hdc,
			layout.errorText,
			p.errorText,
			p.document.fonts.ui,
			colorOptionsError,
			co.DT_LEFT|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_END_ELLIPSIS|
				co.DT_NOPREFIX,
		); err != nil {
			return err
		}
	}
	if err := p.paintButton(
		hdc,
		layout.cancelButton,
		"取消",
		false,
		optionsHitCancel,
		p.document.fonts.ui,
	); err != nil {
		return err
	}
	return p.paintButton(
		hdc,
		layout.saveButton,
		"保存",
		true,
		optionsHitSave,
		p.document.fonts.title,
	)
}

func (p *optionsPanel) paintButton(
	hdc win.HDC,
	rect win.RECT,
	label string,
	primary bool,
	hit optionsPanelHit,
	font win.HFONT,
) error {
	background := colorWindowBackground
	textColor := colorTextPrimary
	borderColor := colorOptionsPanelBorder
	if primary {
		background = colorOptionsAccent
		textColor = colorWindowBackground
		borderColor = colorOptionsAccent
	}
	if p.hover == hit || p.pressed == hit {
		if primary {
			background = colorOptionsAccentStrong
			borderColor = colorOptionsAccentStrong
		} else {
			background = colorOptionsAccentSoft
			borderColor = colorOptionsAccent
		}
	}
	if err := fillOptionsRoundedRect(
		hdc,
		rect,
		optionsButtonRadiusDIP,
		background,
	); err != nil {
		return err
	}
	if err := strokeOptionsRoundedRect(
		hdc,
		rect,
		optionsButtonRadiusDIP,
		borderColor,
	); err != nil {
		return err
	}
	return paintOptionsText(
		hdc,
		rect,
		label,
		font,
		textColor,
		co.DT_CENTER|co.DT_VCENTER|co.DT_SINGLELINE|co.DT_NOPREFIX,
	)
}

func paintOptionsText(
	hdc win.HDC,
	rect win.RECT,
	text string,
	font win.HFONT,
	color win.COLORREF,
	flags co.DT,
) error {
	if text == "" || rect.Right <= rect.Left || rect.Bottom <= rect.Top {
		return nil
	}
	oldFont, err := hdc.SelectObjectFont(font)
	if err != nil {
		return fmt.Errorf("select options text font: %w", err)
	}
	defer func() {
		_, _ = hdc.SelectObjectFont(oldFont)
	}()
	if _, err := hdc.SetBkMode(co.BKMODE_TRANSPARENT); err != nil {
		return fmt.Errorf("set options text background mode: %w", err)
	}
	if _, err := hdc.SetTextColor(color); err != nil {
		return fmt.Errorf("set options text color: %w", err)
	}
	if err := drawOptionsTextW(hdc, text, &rect, flags); err != nil {
		return fmt.Errorf("paint options text %q: %w", text, err)
	}
	return nil
}

func drawOptionsTextW(
	hdc win.HDC,
	text string,
	rect *win.RECT,
	flags co.DT,
) error {
	textUTF16, err := syscall.UTF16FromString(text)
	if err != nil {
		return fmt.Errorf("encode options text as UTF-16: %w", err)
	}
	// Windigo v0.2.7 passes the UTF-8 byte count to DrawTextW as the UTF-16
	// length. That shifts right-aligned and centered CJK text and can break
	// ellipsis measurement, so call the same API with the wide length.
	ret, _, callErr := drawOptionsTextWProc.Call(
		uintptr(hdc),
		uintptr(unsafe.Pointer(&textUTF16[0])),
		uintptr(len(textUTF16)-1),
		uintptr(unsafe.Pointer(rect)),
		uintptr(flags),
	)
	if ret == 0 {
		return fmt.Errorf("DrawTextW failed: %w", callErr)
	}
	return nil
}

func measureTextHeight(hdc win.HDC, font win.HFONT) (int, error) {
	oldFont, err := hdc.SelectObjectFont(font)
	if err != nil {
		return 0, fmt.Errorf("select preview font for measurement: %w", err)
	}
	defer func() {
		_, _ = hdc.SelectObjectFont(oldFont)
	}()
	metrics, err := hdc.GetTextMetrics()
	if err != nil {
		return 0, fmt.Errorf("measure preview text: %w", err)
	}
	return int(metrics.Height), nil
}

func fillOptionsRoundedRect(
	hdc win.HDC,
	rect win.RECT,
	radius int,
	color win.COLORREF,
) error {
	brush, err := brushFor(color)
	if err != nil {
		return err
	}
	oldBrush, err := hdc.SelectObjectBrush(brush)
	if err != nil {
		return fmt.Errorf("select options fill brush: %w", err)
	}
	defer func() {
		_, _ = hdc.SelectObjectBrush(oldBrush)
	}()
	nullPen, err := win.GetStockObject(co.STOCK_NULL_PEN)
	if err != nil {
		return err
	}
	oldPen, err := hdc.SelectObjectPen(win.HPEN(nullPen))
	if err != nil {
		return fmt.Errorf("select options null pen: %w", err)
	}
	defer func() {
		_, _ = hdc.SelectObjectPen(oldPen)
	}()
	if err := hdc.RoundRect(
		rect,
		win.SIZE{Cx: int32(radius * 2), Cy: int32(radius * 2)},
	); err != nil {
		return fmt.Errorf("fill options rounded rectangle: %w", err)
	}
	return nil
}

func strokeOptionsRoundedRect(
	hdc win.HDC,
	rect win.RECT,
	radius int,
	color win.COLORREF,
) error {
	pen, err := win.CreatePen(co.PS_SOLID, 1, color)
	if err != nil {
		return err
	}
	defer func() {
		_ = pen.DeleteObject()
	}()
	oldPen, err := hdc.SelectObjectPen(pen)
	if err != nil {
		return fmt.Errorf("select options border pen: %w", err)
	}
	defer func() {
		_, _ = hdc.SelectObjectPen(oldPen)
	}()
	nullBrush, err := win.GetStockObject(co.STOCK_NULL_BRUSH)
	if err != nil {
		return err
	}
	oldBrush, err := hdc.SelectObjectBrush(win.HBRUSH(nullBrush))
	if err != nil {
		return fmt.Errorf("select options null brush: %w", err)
	}
	defer func() {
		_, _ = hdc.SelectObjectBrush(oldBrush)
	}()
	if err := hdc.RoundRect(
		rect,
		win.SIZE{Cx: int32(radius * 2), Cy: int32(radius * 2)},
	); err != nil {
		return fmt.Errorf("stroke options rounded rectangle: %w", err)
	}
	return nil
}

func fillOptionsEllipse(
	hdc win.HDC,
	rect win.RECT,
	color win.COLORREF,
) error {
	brush, err := brushFor(color)
	if err != nil {
		return err
	}
	oldBrush, err := hdc.SelectObjectBrush(brush)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = hdc.SelectObjectBrush(oldBrush)
	}()
	nullPen, err := win.GetStockObject(co.STOCK_NULL_PEN)
	if err != nil {
		return err
	}
	oldPen, err := hdc.SelectObjectPen(win.HPEN(nullPen))
	if err != nil {
		return err
	}
	defer func() {
		_, _ = hdc.SelectObjectPen(oldPen)
	}()
	return hdc.Ellipse(rect)
}

func strokeOptionsEllipse(
	hdc win.HDC,
	rect win.RECT,
	color win.COLORREF,
	width int,
) error {
	pen, err := win.CreatePen(co.PS_SOLID, width, color)
	if err != nil {
		return err
	}
	defer func() {
		_ = pen.DeleteObject()
	}()
	oldPen, err := hdc.SelectObjectPen(pen)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = hdc.SelectObjectPen(oldPen)
	}()
	nullBrush, err := win.GetStockObject(co.STOCK_NULL_BRUSH)
	if err != nil {
		return err
	}
	oldBrush, err := hdc.SelectObjectBrush(win.HBRUSH(nullBrush))
	if err != nil {
		return err
	}
	defer func() {
		_, _ = hdc.SelectObjectBrush(oldBrush)
	}()
	return hdc.Ellipse(rect)
}

func drawOptionsHorizontalLine(
	hdc win.HDC,
	left, right, y int,
	color win.COLORREF,
) error {
	pen, err := win.CreatePen(co.PS_SOLID, 1, color)
	if err != nil {
		return err
	}
	defer func() {
		_ = pen.DeleteObject()
	}()
	return drawOptionsLine(hdc, pen, left, y, right, y)
}

func drawOptionsLine(
	hdc win.HDC,
	pen win.HPEN,
	x1, y1, x2, y2 int,
) error {
	oldPen, err := hdc.SelectObjectPen(pen)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = hdc.SelectObjectPen(oldPen)
	}()
	if _, err := hdc.MoveToEx(x1, y1); err != nil {
		return err
	}
	return hdc.LineTo(x2, y2)
}

func sliderThumbValue(
	value, minValue, maxValue int,
	rect win.RECT,
	dpi int,
) int {
	left, right := sliderTrackRange(rect, dpi)
	return sliderThumbX(value, minValue, maxValue, left, right)
}

func editorLineSpacingIndex(value int) int {
	for index, spacing := range config.LineSpacingOptions() {
		if spacing == value {
			return index
		}
	}
	return 0
}

func lineSpacingDisplay(value int) string {
	switch value {
	case 100:
		return "1.0×"
	case 125:
		return "1.25×"
	case 150:
		return "1.5×"
	case 200:
		return "2.0×"
	default:
		return strconv.Itoa(value) + "%"
	}
}
