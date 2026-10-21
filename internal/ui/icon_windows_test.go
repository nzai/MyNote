//go:build windows

package ui

import (
	"encoding/binary"
	"testing"
)

func TestSelectIconImageClosestToTarget(t *testing.T) {
	ico := syntheticICO(
		syntheticIconEntry{size: 16, data: []byte{1}},
		syntheticIconEntry{size: 32, data: []byte{2, 2}},
		syntheticIconEntry{size: 48, data: []byte{3, 3, 3}},
	)
	data, width, err := selectIconImage(ico, 32)
	if err != nil {
		t.Fatal(err)
	}
	if width != 32 {
		t.Fatalf("selected width = %d, want 32", width)
	}
	if len(data) != 2 || data[0] != 2 {
		t.Fatalf("selected data = %v, want entry payload", data)
	}
}

func TestSelectIconImageRejectsMalformedDirectory(t *testing.T) {
	if _, _, err := selectIconImage([]byte{1, 2, 3}, 32); err == nil {
		t.Fatal("expected malformed ICO error")
	}
}

type syntheticIconEntry struct {
	size int
	data []byte
}

func syntheticICO(entries ...syntheticIconEntry) []byte {
	headerSize := 6 + 16*len(entries)
	total := headerSize
	for _, entry := range entries {
		total += len(entry.data)
	}
	out := make([]byte, total)
	binary.LittleEndian.PutUint16(out[2:], 1)
	binary.LittleEndian.PutUint16(out[4:], uint16(len(entries)))

	offset := headerSize
	for i, entry := range entries {
		base := 6 + i*16
		out[base] = byte(entry.size)
		out[base+1] = byte(entry.size)
		binary.LittleEndian.PutUint16(out[base+4:], 1)
		binary.LittleEndian.PutUint16(out[base+6:], 32)
		binary.LittleEndian.PutUint32(out[base+8:], uint32(len(entry.data)))
		binary.LittleEndian.PutUint32(out[base+12:], uint32(offset))
		copy(out[offset:], entry.data)
		offset += len(entry.data)
	}
	return out
}
