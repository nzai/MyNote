package clipboard

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

type dibSpec struct {
	width       int32
	height      int32
	bitCount    uint16
	compression uint32
	colorsUsed  uint32
	masks       []uint32
	palette     [][4]byte
	rows        [][]byte
}

func TestDecodeDIB32BitBottomUp(t *testing.T) {
	dib := makeDIB(dibSpec{
		width:    2,
		height:   2,
		bitCount: 32,
		rows: [][]byte{
			{255, 0, 0, 0, 255, 255, 255, 0},
			{0, 0, 255, 0, 0, 255, 0, 0},
		},
	})

	img, err := DecodeDIB(dib)
	if err != nil {
		t.Fatal(err)
	}

	assertPixel(t, img, 0, 0, 255, 0, 0, 255)
	assertPixel(t, img, 1, 0, 0, 255, 0, 255)
	assertPixel(t, img, 0, 1, 0, 0, 255, 255)
	assertPixel(t, img, 1, 1, 255, 255, 255, 255)
}

func TestDecodeDIBRejectsTruncatedPixels(t *testing.T) {
	dib := makeDIB(dibSpec{
		width:    1,
		height:   1,
		bitCount: 32,
		rows:     [][]byte{{0, 0, 255, 0}},
	})
	if _, err := DecodeDIB(dib[:len(dib)-1]); err == nil {
		t.Fatal("expected truncated pixel error")
	}
}

func TestDecodeDIB24BitBottomUpWithRowPadding(t *testing.T) {
	dib := makeDIB(dibSpec{
		width:    2,
		height:   2,
		bitCount: 24,
		rows: [][]byte{
			{255, 0, 0, 255, 255, 255, 0, 0},
			{0, 0, 255, 0, 255, 0, 0, 0},
		},
	})

	img, err := DecodeDIB(dib)
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, img, 0, 0, 255, 0, 0, 255)
	assertPixel(t, img, 1, 0, 0, 255, 0, 255)
	assertPixel(t, img, 0, 1, 0, 0, 255, 255)
	assertPixel(t, img, 1, 1, 255, 255, 255, 255)
}

func TestDecodeDIB16BitRGB555(t *testing.T) {
	dib := makeDIB(dibSpec{
		width:    2,
		height:   1,
		bitCount: 16,
		rows:     [][]byte{{0x00, 0x7c, 0xe0, 0x03}},
	})

	img, err := DecodeDIB(dib)
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, img, 0, 0, 255, 0, 0, 255)
	assertPixel(t, img, 1, 0, 0, 255, 0, 255)
}

func TestDecodeDIB16BitRGB565BitFields(t *testing.T) {
	dib := makeDIB(dibSpec{
		width:       2,
		height:      1,
		bitCount:    16,
		compression: 3,
		masks:       []uint32{0xf800, 0x07e0, 0x001f},
		rows:        [][]byte{{0x00, 0xf8, 0x1f, 0x00}},
	})

	img, err := DecodeDIB(dib)
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, img, 0, 0, 255, 0, 0, 255)
	assertPixel(t, img, 1, 0, 0, 0, 255, 255)
}

func TestDecodeDIB32BitAlphaBitFieldsTopDown(t *testing.T) {
	dib := makeDIB(dibSpec{
		width:       1,
		height:      -1,
		bitCount:    32,
		compression: 6,
		masks:       []uint32{0x00ff0000, 0x0000ff00, 0x000000ff, 0xff000000},
		rows:        [][]byte{{0x00, 0x00, 0xff, 0x80}},
	})

	img, err := DecodeDIB(dib)
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, img, 0, 0, 255, 0, 0, 128)
}

func TestDecodeDIB8BitPalette(t *testing.T) {
	dib := makeDIB(dibSpec{
		width:      2,
		height:     1,
		bitCount:   8,
		colorsUsed: 2,
		palette: [][4]byte{
			{255, 0, 0, 0},
			{0, 0, 255, 0},
		},
		rows: [][]byte{{0, 1, 0, 0}},
	})

	img, err := DecodeDIB(dib)
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, img, 0, 0, 0, 0, 255, 255)
	assertPixel(t, img, 1, 0, 255, 0, 0, 255)
}

func TestEncodePNGRoundTrip(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})

	var buf bytes.Buffer
	if err := EncodePNG(&buf, img); err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	assertPixel(t, decoded, 0, 0, 10, 20, 30, 255)
}

func TestToBGRA(t *testing.T) {
	img := image.NewNRGBA(image.Rect(5, 6, 6, 7))
	img.SetNRGBA(5, 6, color.NRGBA{R: 10, G: 20, B: 30, A: 40})

	pixels, width, height := ToBGRA(img)
	if width != 1 || height != 1 {
		t.Fatalf("dimensions = %dx%d", width, height)
	}
	want := []byte{30, 20, 10, 40}
	if !bytes.Equal(pixels, want) {
		t.Fatalf("pixels = %v, want %v", pixels, want)
	}
}

func makeDIB(spec dibSpec) []byte {
	const headerSize = 40
	maskBytes := 0
	if spec.compression == 3 {
		maskBytes = 12
	}
	if spec.compression == 6 {
		maskBytes = 16
	}
	paletteBytes := len(spec.palette) * 4
	rowBytes := 0
	for _, row := range spec.rows {
		rowBytes += len(row)
	}

	data := make([]byte, headerSize+maskBytes+paletteBytes+rowBytes)
	binary.LittleEndian.PutUint32(data[0:4], headerSize)
	binary.LittleEndian.PutUint32(data[4:8], uint32(spec.width))
	binary.LittleEndian.PutUint32(data[8:12], uint32(spec.height))
	binary.LittleEndian.PutUint16(data[12:14], 1)
	binary.LittleEndian.PutUint16(data[14:16], spec.bitCount)
	binary.LittleEndian.PutUint32(data[16:20], spec.compression)
	binary.LittleEndian.PutUint32(data[32:36], spec.colorsUsed)

	offset := headerSize
	for _, mask := range spec.masks {
		binary.LittleEndian.PutUint32(data[offset:offset+4], mask)
		offset += 4
	}
	if maskBytes > 0 && len(spec.masks)*4 > maskBytes {
		panic("too many masks")
	}
	offset = headerSize + maskBytes
	for _, entry := range spec.palette {
		copy(data[offset:offset+4], entry[:])
		offset += 4
	}
	for _, row := range spec.rows {
		copy(data[offset:], row)
		offset += len(row)
	}
	return data
}

func assertPixel(t *testing.T, img image.Image, x, y int, r, g, b, a uint8) {
	t.Helper()
	got := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	want := color.NRGBA{R: r, G: g, B: b, A: a}
	if got != want {
		t.Fatalf("pixel (%d,%d) = %#v, want %#v", x, y, got, want)
	}
}
