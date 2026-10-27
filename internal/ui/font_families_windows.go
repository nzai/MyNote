//go:build windows

package ui

import (
	"fmt"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

var enumFontFamiliesExW = syscall.NewLazyDLL("gdi32.dll").NewProc(
	"EnumFontFamiliesExW",
)

func systemFontFamilies() ([]string, error) {
	hdc, err := win.HWND(0).GetDC()
	if err != nil {
		return nil, fmt.Errorf("get screen DC for font enumeration: %w", err)
	}
	defer func() {
		_ = win.HWND(0).ReleaseDC(hdc)
	}()

	var families []string
	callback := syscall.NewCallback(
		func(
			logFont *win.LOGFONT,
			_ *win.TEXTMETRIC,
			_ uint32,
			_ uintptr,
		) uintptr {
			if logFont != nil {
				families = append(families, logFont.FaceName())
			}
			return 1
		},
	)
	filter := win.LOGFONT{CharSet: co.CHARSET_DEFAULT}
	result, _, callErr := enumFontFamiliesExW.Call(
		uintptr(hdc),
		uintptr(unsafe.Pointer(&filter)),
		callback,
		0,
		0,
	)
	if result == 0 {
		return nil, fmt.Errorf("enumerate installed font families: %w", callErr)
	}
	return normalizeFontFamilies(families), nil
}

func normalizeFontFamilies(families []string) []string {
	seen := make(map[string]bool, len(families))
	result := make([]string, 0, len(families))
	for _, family := range families {
		family = strings.TrimSpace(family)
		if family == "" || strings.HasPrefix(family, "@") {
			continue
		}
		key := strings.ToLower(family)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, family)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
}

func ensureFontFamily(families []string, selected string) []string {
	selected = strings.TrimSpace(selected)
	if selected == "" {
		return normalizeFontFamilies(families)
	}
	for _, family := range families {
		if strings.EqualFold(family, selected) {
			return normalizeFontFamilies(families)
		}
	}
	return normalizeFontFamilies(append(families, selected))
}
