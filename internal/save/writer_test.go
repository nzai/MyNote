package save

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/nzai/MyNote/internal/core"
)

func TestWriterNormalizesTextToCRLFWithoutBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.txt")
	writer := NewAtomicWriter()

	err := writer.Write(context.Background(), path, core.Snapshot{Kind: core.KindText}, "a\nb\r\nc\rd")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(got, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatal("text output must not contain a UTF-8 BOM")
	}
	if want := []byte("a\r\nb\r\nc\r\nd"); !bytes.Equal(got, want) {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestWriterSavesImageAsPNGAndReplacesDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.png")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 255})
	writer := NewAtomicWriter()

	err := writer.Write(
		context.Background(),
		path,
		core.Snapshot{Kind: core.KindImage, Image: img},
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA); got.B != 30 {
		t.Fatalf("pixel = %#v", got)
	}
}

func TestWriterFailureLeavesDestinationAndRemovesTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.png")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	writer := NewAtomicWriter()

	err := writer.Write(
		context.Background(),
		path,
		core.Snapshot{Kind: core.KindImage},
		"",
	)
	if err == nil {
		t.Fatal("expected image write error")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old" {
		t.Fatalf("destination changed to %q", got)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".*.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary files remain: %v", temps)
	}
}

func TestWriterHonorsCanceledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.txt")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NewAtomicWriter().Write(ctx, path, core.Snapshot{Kind: core.KindText}, "hello")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("destination should not exist, stat error = %v", statErr)
	}
}
