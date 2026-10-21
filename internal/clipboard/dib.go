package clipboard

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"math/bits"
)

func DecodeDIB(data []byte) (image.Image, error) {
	if len(data) < 40 {
		return nil, fmt.Errorf("decode DIB: header truncated: got %d bytes", len(data))
	}

	headerSize := binary.LittleEndian.Uint32(data[0:4])
	if headerSize < 40 || uint64(headerSize) > uint64(len(data)) {
		return nil, fmt.Errorf("decode DIB: invalid header size %d for %d bytes", headerSize, len(data))
	}
	width := int64(int32(binary.LittleEndian.Uint32(data[4:8])))
	rawHeight := int64(int32(binary.LittleEndian.Uint32(data[8:12])))
	planes := binary.LittleEndian.Uint16(data[12:14])
	bitCount := binary.LittleEndian.Uint16(data[14:16])
	compression := binary.LittleEndian.Uint32(data[16:20])
	colorsUsed := binary.LittleEndian.Uint32(data[32:36])

	if width <= 0 {
		return nil, fmt.Errorf("decode DIB: invalid width %d", width)
	}
	if rawHeight == 0 {
		return nil, errors.New("decode DIB: height must not be zero")
	}
	if planes != 1 {
		return nil, fmt.Errorf("decode DIB: invalid plane count %d", planes)
	}
	height := rawHeight
	topDown := height < 0
	if topDown {
		height = -height
	}
	if width > int64(math.MaxInt) || height > int64(math.MaxInt) {
		return nil, fmt.Errorf("decode DIB: dimensions %dx%d exceed platform limits", width, height)
	}
	pixelCount, ok := checkedMul(width, height)
	if !ok || pixelCount > int64(math.MaxInt)/4 {
		return nil, fmt.Errorf("decode DIB: pixel allocation overflow for %dx%d", width, height)
	}

	rowBits, ok := checkedMul(width, int64(bitCount))
	if !ok || rowBits > math.MaxInt64-31 {
		return nil, fmt.Errorf("decode DIB: row size overflow for %dx%d at %d bits", width, height, bitCount)
	}
	stride, ok := checkedMul((rowBits+31)/32, 4)
	if !ok {
		return nil, fmt.Errorf("decode DIB: stride overflow for %dx%d at %d bits", width, height, bitCount)
	}
	pixelBytes, ok := checkedMul(stride, height)
	if !ok {
		return nil, fmt.Errorf("decode DIB: pixel size overflow for %dx%d", width, height)
	}

	pixelOffset, redMask, greenMask, blueMask, alphaMask, err := dibMasks(data, headerSize, compression)
	if err != nil {
		return nil, err
	}

	var palette []color.NRGBA
	if bitCount == 8 && compression == 0 {
		count := int64(colorsUsed)
		if count == 0 {
			count = 256
		}
		if count > 256 {
			return nil, fmt.Errorf("decode DIB: palette has %d entries, maximum is 256", count)
		}
		paletteBytes, ok := checkedMul(count, 4)
		if !ok || pixelOffset > math.MaxInt64-paletteBytes {
			return nil, fmt.Errorf("decode DIB: palette size overflow: %d entries", count)
		}
		if pixelOffset+paletteBytes > int64(len(data)) {
			return nil, fmt.Errorf(
				"decode DIB: palette truncated: need %d bytes at offset %d, have %d",
				paletteBytes,
				pixelOffset,
				len(data),
			)
		}
		palette = make([]color.NRGBA, int(count))
		for index := range palette {
			offset := int(pixelOffset) + index*4
			palette[index] = color.NRGBA{
				R: data[offset+2],
				G: data[offset+1],
				B: data[offset],
				A: 255,
			}
		}
		pixelOffset += paletteBytes
	} else if bitCount != 16 && bitCount != 24 && bitCount != 32 {
		return nil, fmt.Errorf("decode DIB: unsupported format %d bits, compression %d", bitCount, compression)
	}
	if compression != 0 && compression != 3 && compression != 6 {
		return nil, fmt.Errorf("decode DIB: unsupported compression %d", compression)
	}

	if pixelOffset+pixelBytes > int64(len(data)) {
		return nil, fmt.Errorf(
			"decode DIB: pixel data truncated: need %d bytes at offset %d, have %d",
			pixelBytes,
			pixelOffset,
			len(data),
		)
	}

	img := image.NewNRGBA(image.Rect(0, 0, int(width), int(height)))
	var redField, greenField, blueField, alphaField maskField
	switch {
	case compression == 3 || compression == 6:
		redField, err = newMaskField(redMask)
		if err == nil {
			greenField, err = newMaskField(greenMask)
		}
		if err == nil {
			blueField, err = newMaskField(blueMask)
		}
		if err != nil {
			return nil, fmt.Errorf("decode DIB: invalid color mask: %w", err)
		}
		if alphaMask != 0 {
			alphaField, err = newMaskField(alphaMask)
			if err != nil {
				return nil, fmt.Errorf("decode DIB: invalid alpha mask: %w", err)
			}
		}
	case bitCount == 16:
		redField = maskField{mask: 0x7c00, shift: 10, max: 31}
		greenField = maskField{mask: 0x03e0, shift: 5, max: 31}
		blueField = maskField{mask: 0x001f, max: 31}
	}

	hasAlpha := bitCount == 32 && compression == 0 || alphaMask != 0
	anyAlpha := false
	for y := 0; y < int(height); y++ {
		sourceY := y
		if !topDown {
			sourceY = int(height) - 1 - y
		}
		row := data[int(pixelOffset)+sourceY*int(stride):]
		for x := 0; x < int(width); x++ {
			target := y*img.Stride + x*4
			switch bitCount {
			case 8:
				index := int(row[x])
				if index >= len(palette) {
					return nil, fmt.Errorf("decode DIB: palette index %d out of range for %d entries", index, len(palette))
				}
				pixel := palette[index]
				img.Pix[target] = pixel.R
				img.Pix[target+1] = pixel.G
				img.Pix[target+2] = pixel.B
				img.Pix[target+3] = 255
			case 16:
				pixel := uint32(binary.LittleEndian.Uint16(row[x*2:]))
				img.Pix[target] = redField.value(pixel)
				img.Pix[target+1] = greenField.value(pixel)
				img.Pix[target+2] = blueField.value(pixel)
				if hasAlpha {
					rawAlpha := alphaField.raw(pixel)
					img.Pix[target+3] = alphaField.value(pixel)
					anyAlpha = anyAlpha || rawAlpha != 0
				} else {
					img.Pix[target+3] = 255
				}
			case 24:
				source := row[x*3:]
				img.Pix[target] = source[2]
				img.Pix[target+1] = source[1]
				img.Pix[target+2] = source[0]
				img.Pix[target+3] = 255
			case 32:
				source := row[x*4:]
				if compression == 0 {
					img.Pix[target] = source[2]
					img.Pix[target+1] = source[1]
					img.Pix[target+2] = source[0]
					img.Pix[target+3] = source[3]
					anyAlpha = anyAlpha || source[3] != 0
				} else {
					pixel := binary.LittleEndian.Uint32(source)
					img.Pix[target] = redField.value(pixel)
					img.Pix[target+1] = greenField.value(pixel)
					img.Pix[target+2] = blueField.value(pixel)
					if hasAlpha {
						rawAlpha := alphaField.raw(pixel)
						img.Pix[target+3] = alphaField.value(pixel)
						anyAlpha = anyAlpha || rawAlpha != 0
					} else {
						img.Pix[target+3] = 255
					}
				}
			}
		}
	}
	if hasAlpha && !anyAlpha {
		for offset := 3; offset < len(img.Pix); offset += 4 {
			img.Pix[offset] = 255
		}
	}
	return img, nil
}

func EncodePNG(w io.Writer, img image.Image) error {
	if err := png.Encode(w, img); err != nil {
		return fmt.Errorf("encode PNG: %w", err)
	}
	return nil
}

func ToBGRA(img image.Image) ([]byte, int, int) {
	if img == nil {
		return nil, 0, 0
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pixels := make([]byte, width*height*4)
	offset := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			pixels[offset] = pixel.B
			pixels[offset+1] = pixel.G
			pixels[offset+2] = pixel.R
			pixels[offset+3] = pixel.A
			offset += 4
		}
	}
	return pixels, width, height
}

type maskField struct {
	mask  uint32
	shift uint
	max   uint32
}

func newMaskField(mask uint32) (maskField, error) {
	if mask == 0 {
		return maskField{}, errors.New("mask is zero")
	}
	shift := bits.TrailingZeros32(mask)
	normalized := mask >> shift
	if normalized&(normalized+1) != 0 {
		return maskField{}, fmt.Errorf("mask %#08x is not contiguous", mask)
	}
	return maskField{mask: mask, shift: uint(shift), max: normalized}, nil
}

func (m maskField) raw(pixel uint32) uint32 {
	if m.mask == 0 {
		return 0
	}
	return (pixel & m.mask) >> m.shift
}

func (m maskField) value(pixel uint32) uint8 {
	if m.mask == 0 || m.max == 0 {
		return 0
	}
	raw := uint64(m.raw(pixel))
	return uint8((raw*255 + uint64(m.max)/2) / uint64(m.max))
}

func dibMasks(
	data []byte,
	headerSize uint32,
	compression uint32,
) (pixelOffset int64, red, green, blue, alpha uint32, err error) {
	pixelOffset = int64(headerSize)
	switch compression {
	case 0:
		return pixelOffset, 0, 0, 0, 0, nil
	case 3:
		if headerSize >= 52 {
			return pixelOffset,
				binary.LittleEndian.Uint32(data[40:44]),
				binary.LittleEndian.Uint32(data[44:48]),
				binary.LittleEndian.Uint32(data[48:52]),
				0,
				nil
		}
		offset := int64(headerSize)
		if offset+12 > int64(len(data)) {
			return 0, 0, 0, 0, 0, errors.New("decode DIB: bitfield masks truncated")
		}
		return offset + 12,
			binary.LittleEndian.Uint32(data[offset : offset+4]),
			binary.LittleEndian.Uint32(data[offset+4 : offset+8]),
			binary.LittleEndian.Uint32(data[offset+8 : offset+12]),
			0,
			nil
	case 6:
		if headerSize >= 56 {
			return pixelOffset,
				binary.LittleEndian.Uint32(data[40:44]),
				binary.LittleEndian.Uint32(data[44:48]),
				binary.LittleEndian.Uint32(data[48:52]),
				binary.LittleEndian.Uint32(data[52:56]),
				nil
		}
		offset := int64(headerSize)
		if offset+16 > int64(len(data)) {
			return 0, 0, 0, 0, 0, errors.New("decode DIB: alpha bitfield masks truncated")
		}
		return offset + 16,
			binary.LittleEndian.Uint32(data[offset : offset+4]),
			binary.LittleEndian.Uint32(data[offset+4 : offset+8]),
			binary.LittleEndian.Uint32(data[offset+8 : offset+12]),
			binary.LittleEndian.Uint32(data[offset+12 : offset+16]),
			nil
	default:
		return pixelOffset, 0, 0, 0, 0, fmt.Errorf("decode DIB: unsupported compression %d", compression)
	}
}

func checkedMul(left, right int64) (int64, bool) {
	if left == 0 || right == 0 {
		return 0, true
	}
	if left > math.MaxInt64/right {
		return 0, false
	}
	return left * right, true
}
