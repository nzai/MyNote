//go:build windows

package ui

import (
	"image"
	"image/color"
	"testing"
)

func TestImageFitRect(t *testing.T) {
	tests := []struct {
		name             string
		srcW, srcH       int
		dstW, dstH       int
		wantX, wantY     int
		wantWidth, wantH int
	}{
		{
			name: "wide image",
			srcW: 800, srcH: 600, dstW: 400, dstH: 400,
			wantX: 0, wantY: 50, wantWidth: 400, wantH: 300,
		},
		{
			name: "tall image",
			srcW: 100, srcH: 200, dstW: 300, dstH: 100,
			wantX: 125, wantY: 0, wantWidth: 50, wantH: 100,
		},
		{
			name: "empty target",
			srcW: 100, srcH: 100, dstW: 0, dstH: 100,
			wantX: 0, wantY: 0, wantWidth: 0, wantH: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := imageFitRect(
				test.srcW,
				test.srcH,
				test.dstW,
				test.dstH,
			)
			if got.X != test.wantX || got.Y != test.wantY ||
				got.Width != test.wantWidth || got.Height != test.wantH {
				t.Fatalf("imageFitRect() = %+v, want x=%d y=%d w=%d h=%d",
					got,
					test.wantX,
					test.wantY,
					test.wantWidth,
					test.wantH,
				)
			}
		})
	}
}

func TestRasterizeImageBGRA(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{R: 0, G: 0, B: 255, A: 128})

	got, err := rasterizeImage(img)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{
		0, 0, 255, 255,
		255, 0, 0, 128,
	}
	if got.Width != 2 || got.Height != 1 {
		t.Fatalf("raster size = %dx%d, want 2x1", got.Width, got.Height)
	}
	if len(got.Pixels) != len(want) {
		t.Fatalf("pixel bytes = %d, want %d", len(got.Pixels), len(want))
	}
	for i := range want {
		if got.Pixels[i] != want[i] {
			t.Fatalf("pixel byte %d = %d, want %d", i, got.Pixels[i], want[i])
		}
	}
}
