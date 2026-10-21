package assets

import (
	"encoding/binary"
	"slices"
	"testing"
)

func TestAppICOContainsIconSizesForWindowsScaling(t *testing.T) {
	const (
		iconDirectoryHeaderSize = 6
		iconDirectoryEntrySize  = 16
	)

	if len(AppICO) < iconDirectoryHeaderSize {
		t.Fatalf("icon is %d bytes, want at least %d", len(AppICO), iconDirectoryHeaderSize)
	}
	if reserved := binary.LittleEndian.Uint16(AppICO[0:2]); reserved != 0 {
		t.Fatalf("icon reserved field = %d, want 0", reserved)
	}
	if iconType := binary.LittleEndian.Uint16(AppICO[2:4]); iconType != 1 {
		t.Fatalf("icon type = %d, want 1", iconType)
	}

	count := int(binary.LittleEndian.Uint16(AppICO[4:6]))
	directoryEnd := iconDirectoryHeaderSize + count*iconDirectoryEntrySize
	if directoryEnd > len(AppICO) {
		t.Fatalf("icon directory needs %d bytes, file has %d", directoryEnd, len(AppICO))
	}

	sizes := make([]int, 0, count)
	for index := 0; index < count; index++ {
		entry := AppICO[iconDirectoryHeaderSize+index*iconDirectoryEntrySize : iconDirectoryHeaderSize+(index+1)*iconDirectoryEntrySize]
		width := int(entry[0])
		height := int(entry[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		if width != height {
			t.Errorf("icon image %d is %dx%d, want square", index, width, height)
		}

		payloadSize := int(binary.LittleEndian.Uint32(entry[8:12]))
		payloadOffset := int(binary.LittleEndian.Uint32(entry[12:16]))
		if payloadSize <= 0 || payloadOffset < directoryEnd || payloadOffset > len(AppICO)-payloadSize {
			t.Errorf("icon image %d has invalid payload bounds: offset=%d size=%d", index, payloadOffset, payloadSize)
		}
		sizes = append(sizes, width)
	}

	want := []int{16, 20, 24, 32, 40, 48, 64, 128, 256}
	for _, size := range want {
		if !slices.Contains(sizes, size) {
			t.Errorf("icon images = %v, missing %dpx image", sizes, size)
		}
	}
}
