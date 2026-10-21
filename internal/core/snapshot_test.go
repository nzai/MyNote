package core

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestChooseFormatPrefersText(t *testing.T) {
	got, source, err := ChooseFormat(Formats{HasText: true, HasDIBV5: true, HasDIB: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != KindText || source != "CF_UNICODETEXT" {
		t.Fatalf("got %v %q", got, source)
	}
}

func TestChooseFormatPrefersDIBV5OverDIB(t *testing.T) {
	got, source, err := ChooseFormat(Formats{HasDIBV5: true, HasDIB: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != KindImage || source != "CF_DIBV5" {
		t.Fatalf("got %v %q", got, source)
	}
}

func TestChooseFormatReturnsUnsupported(t *testing.T) {
	_, _, err := ChooseFormat(Formats{})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("got %v, want ErrUnsupported", err)
	}
}

func TestValidateSizeAcceptsExactLimit(t *testing.T) {
	if err := ValidateSize(KindText, 32, Limits{MaxTextBytes: 32}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateSizeRejectsOversizedText(t *testing.T) {
	err := ValidateSize(KindText, 33, Limits{MaxTextBytes: 32})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v, want ErrTooLarge", err)
	}
}

func TestValidateSizeRejectsOversizedImage(t *testing.T) {
	err := ValidateSize(KindImage, 129, Limits{MaxImageBytes: 128})
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v, want ErrTooLarge", err)
	}
}

func TestNewCaptureIDIncludesLocalTimestampAndRandomSuffix(t *testing.T) {
	at := time.Date(2026, 10, 21, 15, 30, 12, 987, time.Local)
	got, err := NewCaptureID(at, bytes.NewReader([]byte{0x7f, 0x3a, 0x00, 0xff}))
	if err != nil {
		t.Fatal(err)
	}
	if want := "20261021T153012-7f3a00ff"; got != want {
		t.Fatalf("capture ID = %q, want %q", got, want)
	}
}

func TestNewCaptureIDReportsShortRandomSource(t *testing.T) {
	_, err := NewCaptureID(time.Now(), strings.NewReader("x"))
	if err == nil {
		t.Fatal("expected random source error")
	}
}
