//go:build windows

package ui

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const iconResourceVersion = 0x0003_0000

func selectIconImage(data []byte, targetWidth int) ([]byte, int, error) {
	if len(data) < 6 {
		return nil, 0, errors.New("parse ICO: header is truncated")
	}
	if binary.LittleEndian.Uint16(data[0:2]) != 0 ||
		binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return nil, 0, errors.New("parse ICO: invalid header signature")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count == 0 {
		return nil, 0, errors.New("parse ICO: no images")
	}
	directoryEnd := 6 + count*16
	if directoryEnd > len(data) {
		return nil, 0, errors.New("parse ICO: image directory is truncated")
	}

	bestIndex := -1
	bestDistance := 1 << 30
	bestWidth := 0
	for index := 0; index < count; index++ {
		entry := data[6+index*16 : 6+(index+1)*16]
		width := int(entry[0])
		if width == 0 {
			width = 256
		}
		size := int(binary.LittleEndian.Uint32(entry[8:12]))
		offset := int(binary.LittleEndian.Uint32(entry[12:16]))
		if size <= 0 || offset < directoryEnd || offset > len(data)-size {
			return nil, 0, fmt.Errorf("parse ICO: image %d has invalid payload bounds", index)
		}
		distance := width - targetWidth
		if distance < 0 {
			distance = -distance
		}
		if distance < bestDistance || (distance == bestDistance && width > bestWidth) {
			bestIndex = index
			bestDistance = distance
			bestWidth = width
		}
	}
	if bestIndex < 0 {
		return nil, 0, errors.New("parse ICO: no image selected")
	}

	entry := data[6+bestIndex*16 : 6+(bestIndex+1)*16]
	size := int(binary.LittleEndian.Uint32(entry[8:12]))
	offset := int(binary.LittleEndian.Uint32(entry[12:16]))
	return data[offset : offset+size], bestWidth, nil
}

func loadAppIcon(data []byte) (win.HICON, error) {
	selected, width, err := selectIconImage(data, 32)
	if err != nil {
		return 0, err
	}
	icon, err := win.CreateIconFromResourceEx(
		selected,
		iconResourceVersion,
		win.SIZE{Cx: 32, Cy: 32},
		co.LR_DEFAULTCOLOR,
	)
	if err != nil {
		return 0, fmt.Errorf("create app icon from %dpx resource: %w", width, err)
	}
	return icon, nil
}

func loadStockAppIcon() (win.HICON, error) {
	icon, err := win.HINSTANCE(0).LoadIcon(win.IconResIdi(co.IDI_APPLICATION))
	if err != nil {
		return 0, fmt.Errorf("load stock application icon: %w", err)
	}
	return icon, nil
}
