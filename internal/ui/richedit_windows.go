//go:build windows

package ui

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/nzai/MyNote/internal/config"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/wstr"
)

const (
	richEditClassName = "RICHEDIT50W"

	emSetCharFormat co.WM = co.WM_USER + 68
	emSetParaFormat co.WM = co.WM_USER + 71
	emSetEventMask  co.WM = co.WM_USER + 69
	emSetOptions    co.WM = co.WM_USER + 77

	scfDefault   = 0x0000
	scfSelection = 0x0001
	scfAll       = 0x0004

	cfmBold      uint32 = 0x00000001
	cfmItalic    uint32 = 0x00000002
	cfmUnderline uint32 = 0x00000004
	cfmStrikeout uint32 = 0x00000008
	cfmFace      uint32 = 0x20000000
	cfmSize      uint32 = 0x80000000
	cfmColor     uint32 = 0x40000000
	cfmWeight    uint32 = 0x00400000
	cfmCharSet   uint32 = 0x08000000

	pfmLineSpacing uint32 = 0x00000100

	lineSpacingMultiple = 5

	enmChange = 0x00000001
	enmScroll = 0x00000008

	ecoAutoWordSelection = 0x00000001
	ecoOpOr              = 0x0002
)

var (
	richEditLibraryOnce sync.Once
	richEditLibraryErr  error
)

type richEditCharFormat struct {
	CbSize        uint32
	DwMask        uint32
	DwEffects     uint32
	YHeight       int32
	YOffset       int32
	CrTextColor   uint32
	BCharSet      uint8
	BPitchAndFam  uint8
	SzFaceName    [32]uint16
	WWeight       uint16
	SSpacing      int16
	CrBackColor   uint32
	Lcid          uint32
	DwReserved    uint32
	SStyle        int16
	WKerning      uint16
	BUnderlineTyp uint8
	BAnimation    uint8
	BRevAuthor    uint8
	BReserved1    uint8
}

type richEditParaFormat struct {
	CbSize           uint32
	DwMask           uint32
	WNumbering       uint16
	WReserved        uint16
	DxStartIndent    int32
	DxRightIndent    int32
	DxOffset         int32
	WAlignment       uint16
	CTabCount        int16
	RgxTabs          [32]int32
	DySpaceBefore    int32
	DySpaceAfter     int32
	DyLineSpacing    int32
	SStyle           int16
	BLineSpacingRule uint8
	BOutlineLevel    uint8
	WShadingWeight   uint16
	WShadingStyle    uint16
	WNumberingStart  uint16
	WNumberingStyle  uint16
	WNumberingTab    uint16
	WBorderSpace     uint16
	WBorderWidth     uint16
	WBorders         uint16
}

func ensureRichEditLibrary() error {
	richEditLibraryOnce.Do(func() {
		// Msftedit.dll registers RICHEDIT50W. Keeping the library loaded for the
		// process lifetime avoids invalidating the class while windows still exist.
		if _, err := win.LoadLibrary("Msftedit.dll"); err != nil {
			richEditLibraryErr = fmt.Errorf("load Msftedit.dll: %w", err)
		}
	})
	return richEditLibraryErr
}

func richEditCharacterFormat(settings config.EditorConfig) richEditCharFormat {
	format := richEditCharFormat{
		CbSize:      uint32(richEditCharFormatSize()),
		DwMask:      cfmFace | cfmSize | cfmColor | cfmWeight | cfmCharSet,
		YHeight:     int32(settings.FontPointSize * 20),
		CrTextColor: uint32(colorTextPrimary),
		BCharSet:    uint8(co.CHARSET_DEFAULT),
		WWeight:     400,
	}
	wstr.EncodeToBuf(format.SzFaceName[:], settings.FontFamily)
	return format
}

func richEditParagraphFormat(lineSpacing int) richEditParaFormat {
	format := richEditParaFormat{
		CbSize:           uint32(richEditParaFormatSize()),
		DwMask:           pfmLineSpacing,
		BLineSpacingRule: lineSpacingMultiple,
		DyLineSpacing:    int32(lineSpacing / 5),
	}
	return format
}

func richEditCharFormatSize() uintptr {
	return unsafe.Sizeof(richEditCharFormat{})
}

func richEditParaFormatSize() uintptr {
	return unsafe.Sizeof(richEditParaFormat{})
}

func faceNameFromRichEditFormat(format *richEditCharFormat) string {
	return wstr.DecodeSlice(format.SzFaceName[:])
}

func configureRichEdit(hwnd win.HWND, settings config.EditorConfig) error {
	if hwnd == 0 {
		return errors.New("configure RichEdit: window is nil")
	}
	if err := settings.Validate(); err != nil {
		return fmt.Errorf("configure RichEdit: %w", err)
	}

	if _, err := hwnd.SendMessage(
		emSetEventMask,
		win.WPARAM(enmChange|enmScroll),
		0,
	); err != nil {
		return fmt.Errorf("configure RichEdit event mask: %w", err)
	}
	if _, err := hwnd.SendMessage(
		emSetOptions,
		win.WPARAM(ecoOpOr),
		win.LPARAM(ecoAutoWordSelection),
	); err != nil {
		return fmt.Errorf("configure RichEdit options: %w", err)
	}

	charFormat := richEditCharacterFormat(settings)
	for _, mask := range []uintptr{scfAll, scfDefault} {
		if _, err := hwnd.SendMessage(
			emSetCharFormat,
			win.WPARAM(mask),
			win.LPARAM(uintptr(unsafe.Pointer(&charFormat))),
		); err != nil {
			return fmt.Errorf("apply RichEdit font: %w", err)
		}
	}

	paraFormat := richEditParagraphFormat(settings.LineSpacing)
	for _, mask := range []uintptr{scfAll, scfDefault} {
		if _, err := hwnd.SendMessage(
			emSetParaFormat,
			win.WPARAM(mask),
			win.LPARAM(uintptr(unsafe.Pointer(&paraFormat))),
		); err != nil {
			return fmt.Errorf("apply RichEdit line spacing: %w", err)
		}
	}
	return nil
}
