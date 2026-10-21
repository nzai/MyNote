//go:build windows

package ui

import (
	"errors"
	"fmt"
	"sync"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const (
	uiFontPointSize    = 10
	titleFontPointSize = 11
	iconFontPointSize  = 13
)

type documentFonts struct {
	ui    win.HFONT
	title win.HFONT
	icon  win.HFONT
}

var resourceCache = struct {
	sync.Mutex
	fonts   map[int]documentFonts
	brushes map[win.COLORREF]win.HBRUSH
}{
	fonts:   make(map[int]documentFonts),
	brushes: make(map[win.COLORREF]win.HBRUSH),
}

func fontsForDPI(dpi int) (documentFonts, error) {
	resourceCache.Lock()
	defer resourceCache.Unlock()

	if fonts, ok := resourceCache.fonts[dpi]; ok {
		return fonts, nil
	}
	ui, err := createUIFont(uiFontPointSize, dpi, "Segoe UI", int(co.FW_NORMAL))
	if err != nil {
		return documentFonts{}, err
	}
	title, err := createUIFont(titleFontPointSize, dpi, "Segoe UI", int(co.FW_SEMIBOLD))
	if err != nil {
		_ = ui.DeleteObject()
		return documentFonts{}, err
	}
	icon, err := createUIFont(iconFontPointSize, dpi, "Segoe MDL2 Assets", int(co.FW_NORMAL))
	if err != nil {
		_ = title.DeleteObject()
		_ = ui.DeleteObject()
		return documentFonts{}, err
	}

	fonts := documentFonts{ui: ui, title: title, icon: icon}
	resourceCache.fonts[dpi] = fonts
	return fonts, nil
}

func createUIFont(pointSize, dpi int, face string, weight int) (win.HFONT, error) {
	height := -(pointSize*dpi + 36) / 72
	font, err := win.CreateFont(
		height,
		0,
		0,
		0,
		weight,
		false,
		false,
		false,
		co.CHARSET_DEFAULT,
		co.OUT_PRECIS_DEFAULT,
		co.CLIP_PRECIS_DEFAULT,
		co.QUALITY_CLEARTYPE,
		co.PITCH_DEFAULT,
		co.FF_DONTCARE,
		face,
	)
	if err != nil {
		return 0, fmt.Errorf("create GDI font %q at %d pt for %d DPI: %w", face, pointSize, dpi, err)
	}
	return font, nil
}

func brushFor(color win.COLORREF) (win.HBRUSH, error) {
	resourceCache.Lock()
	defer resourceCache.Unlock()

	if brush, ok := resourceCache.brushes[color]; ok {
		return brush, nil
	}
	brush, err := win.CreateBrushIndirect(&win.LOGBRUSH{
		Style: co.BRS_SOLID,
		Color: color,
	})
	if err != nil {
		return 0, fmt.Errorf("create GDI brush %#08x: %w", uint32(color), err)
	}
	resourceCache.brushes[color] = brush
	return brush, nil
}

func releaseUIResources() error {
	resourceCache.Lock()
	defer resourceCache.Unlock()

	var failures []error
	for dpi, fonts := range resourceCache.fonts {
		if err := fonts.ui.DeleteObject(); err != nil {
			failures = append(failures, fmt.Errorf("delete UI font for %d DPI: %w", dpi, err))
		}
		if err := fonts.title.DeleteObject(); err != nil {
			failures = append(failures, fmt.Errorf("delete title font for %d DPI: %w", dpi, err))
		}
		if err := fonts.icon.DeleteObject(); err != nil {
			failures = append(failures, fmt.Errorf("delete icon font for %d DPI: %w", dpi, err))
		}
	}
	for color, brush := range resourceCache.brushes {
		if err := brush.DeleteObject(); err != nil {
			failures = append(failures, fmt.Errorf("delete brush %#08x: %w", uint32(color), err))
		}
	}
	resourceCache.fonts = make(map[int]documentFonts)
	resourceCache.brushes = make(map[win.COLORREF]win.HBRUSH)
	return errors.Join(failures...)
}

func colorRef(red, green, blue byte) win.COLORREF {
	return win.COLORREF(uint32(red) | uint32(green)<<8 | uint32(blue)<<16)
}
