//go:build windows

package ui

import (
	"errors"
	"fmt"
	"image"
	"image/color"

	"github.com/nzai/MyNote/internal/core"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

type imageRaster struct {
	Width  int
	Height int
	Pixels []byte
}

func (d *Document) paintImageContent(
	hdc win.HDC,
	width, height, titleHeight, statusHeight, dpi int,
) error {
	if d.snapshot.Kind != core.KindImage {
		return nil
	}
	contentTop := titleHeight
	contentBottom := height - statusHeight
	if width <= 0 || contentBottom <= contentTop {
		return nil
	}

	light, err := brushFor(colorCheckerLight)
	if err != nil {
		return err
	}
	dark, err := brushFor(colorCheckerDark)
	if err != nil {
		return err
	}
	if err := hdc.FillRect(&win.RECT{
		Left: 0, Top: int32(contentTop), Right: int32(width), Bottom: int32(contentBottom),
	}, light); err != nil {
		return fmt.Errorf("paint image background: %w", err)
	}

	checkerSize := scaleDIP(imageCheckerSizeDIP, dpi)
	for top := contentTop; top < contentBottom; top += checkerSize {
		for left := 0; left < width; left += checkerSize {
			if ((left/checkerSize)+(top/checkerSize))%2 == 0 {
				continue
			}
			bottom := min(top+checkerSize, contentBottom)
			right := min(left+checkerSize, width)
			if err := hdc.FillRect(&win.RECT{
				Left: int32(left), Top: int32(top), Right: int32(right), Bottom: int32(bottom),
			}, dark); err != nil {
				return fmt.Errorf("paint checkerboard: %w", err)
			}
		}
	}

	padding := scaleDIP(contentPaddingDIP, dpi)
	availableWidth := width - 2*padding
	availableHeight := contentBottom - contentTop - 2*padding
	layout := imageFitRect(d.image.Width, d.image.Height, availableWidth, availableHeight)
	if layout.Width == 0 || layout.Height == 0 {
		return nil
	}

	var bitmapInfo win.BITMAPINFO
	bitmapInfo.BmiHeader.Width = int32(d.image.Width)
	bitmapInfo.BmiHeader.Height = -int32(d.image.Height)
	bitmapInfo.BmiHeader.Planes = 1
	bitmapInfo.BmiHeader.BitCount = co.BITCOUNT_32
	bitmapInfo.BmiHeader.Compression = co.BI_RGB
	bitmapInfo.BmiHeader.SizeImage = uint32(len(d.image.Pixels))
	bitmapInfo.BmiHeader.SetBiSize()
	if _, err := hdc.StretchDIBits(
		win.POINT{X: int32(padding + layout.X), Y: int32(contentTop + padding + layout.Y)},
		win.SIZE{Cx: int32(layout.Width), Cy: int32(layout.Height)},
		win.POINT{},
		win.SIZE{Cx: int32(d.image.Width), Cy: int32(d.image.Height)},
		&d.image.Pixels[0],
		&bitmapInfo,
		co.DIB_COLORS_RGB,
		co.ROP_SRCCOPY,
	); err != nil {
		return fmt.Errorf("paint image pixels: %w", err)
	}
	return nil
}

type imageLayout struct {
	X      int
	Y      int
	Width  int
	Height int
}

func rasterizeImage(source image.Image) (imageRaster, error) {
	if source == nil {
		return imageRaster{}, errors.New("rasterize image: source is nil")
	}
	bounds := source.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return imageRaster{}, fmt.Errorf("rasterize image: invalid dimensions %dx%d", width, height)
	}
	maxInt := int(^uint(0) >> 1)
	if width > maxInt/height/4 {
		return imageRaster{}, fmt.Errorf("rasterize image: dimensions %dx%d overflow", width, height)
	}

	pixels := make([]byte, width*height*4)
	offset := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pixel := color.NRGBAModel.Convert(source.At(x, y)).(color.NRGBA)
			pixels[offset] = pixel.B
			pixels[offset+1] = pixel.G
			pixels[offset+2] = pixel.R
			pixels[offset+3] = pixel.A
			offset += 4
		}
	}
	return imageRaster{Width: width, Height: height, Pixels: pixels}, nil
}

func imageFitRect(sourceWidth, sourceHeight, targetWidth, targetHeight int) imageLayout {
	if sourceWidth <= 0 || sourceHeight <= 0 || targetWidth <= 0 || targetHeight <= 0 {
		return imageLayout{}
	}

	width := targetWidth
	height := int(int64(sourceHeight) * int64(targetWidth) / int64(sourceWidth))
	if height > targetHeight {
		height = targetHeight
		width = int(int64(sourceWidth) * int64(targetHeight) / int64(sourceHeight))
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return imageLayout{
		X:      (targetWidth - width) / 2,
		Y:      (targetHeight - height) / 2,
		Width:  width,
		Height: height,
	}
}
