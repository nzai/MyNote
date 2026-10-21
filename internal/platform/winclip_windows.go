//go:build windows

package platform

import (
	"errors"
	"fmt"

	"github.com/nzai/MyNote/internal/clipboard"
	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

type winClipboardOpener struct {
	owner win.HWND
}

type winClipboardSession struct {
	handle       win.HCLIPBOARD
	available    map[core.Format]bool
	hasAnyFormat bool
}

var (
	_ clipboard.Opener  = winClipboardOpener{}
	_ clipboard.Session = (*winClipboardSession)(nil)
)

func NewWinClipboardOpener(owner win.HWND) clipboard.Opener {
	return winClipboardOpener{owner: owner}
}

func (o winClipboardOpener) Open() (clipboard.Session, error) {
	handle, err := win.OpenClipboard(o.owner)
	if err != nil {
		return nil, fmt.Errorf("open Win32 clipboard owned by hwnd %#x: %w", uintptr(o.owner), err)
	}

	available := make(map[core.Format]bool, 3)
	for _, format := range []core.Format{core.FormatText, core.FormatDIBV5, core.FormatDIB} {
		winFormat, ok := winClipboardFormat(format)
		if !ok {
			return nil, o.closeFailedOpen(handle, fmt.Errorf("map clipboard format %d", format))
		}
		isAvailable, err := handle.IsClipboardFormatAvailable(winFormat)
		if err != nil {
			return nil, o.closeFailedOpen(
				handle,
				fmt.Errorf("query Win32 clipboard format %v: %w", winFormat, err),
			)
		}
		available[format] = isAvailable
	}
	formatCount, err := handle.CountClipboardFormats()
	if err != nil {
		return nil, o.closeFailedOpen(
			handle,
			fmt.Errorf("count Win32 clipboard formats: %w", err),
		)
	}
	return &winClipboardSession{
		handle:       handle,
		available:    available,
		hasAnyFormat: formatCount > 0,
	}, nil
}

func (s *winClipboardSession) Available(format core.Format) bool {
	return s.available[format]
}

func (s *winClipboardSession) HasAnyFormat() bool {
	return s.hasAnyFormat
}

func (s *winClipboardSession) Read(format core.Format) ([]byte, error) {
	winFormat, ok := winClipboardFormat(format)
	if !ok {
		return nil, fmt.Errorf("read Win32 clipboard: unknown format %d", format)
	}
	if !s.available[format] {
		return nil, fmt.Errorf("read Win32 clipboard format %v: format is unavailable", winFormat)
	}
	data, err := s.handle.GetClipboardData(winFormat)
	if err != nil {
		return nil, fmt.Errorf("read Win32 clipboard format %v: %w", winFormat, err)
	}
	return data, nil
}

func (s *winClipboardSession) Close() error {
	if err := s.handle.CloseClipboard(); err != nil {
		return fmt.Errorf("close Win32 clipboard: %w", err)
	}
	return nil
}

func (o winClipboardOpener) closeFailedOpen(handle win.HCLIPBOARD, primary error) error {
	if err := handle.CloseClipboard(); err != nil {
		return errors.Join(primary, fmt.Errorf("close Win32 clipboard after open failure: %w", err))
	}
	return primary
}

func winClipboardFormat(format core.Format) (co.CF, bool) {
	switch format {
	case core.FormatText:
		return co.CF_UNICODETEXT, true
	case core.FormatDIBV5:
		return co.CF_DIBV5, true
	case core.FormatDIB:
		return co.CF_DIB, true
	default:
		return 0, false
	}
}
