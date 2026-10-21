package core

import (
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"time"
)

var (
	ErrUnsupported = errors.New("unsupported clipboard format")
	ErrTooLarge    = errors.New("clipboard content too large")
)

type Kind uint8

const (
	KindText Kind = iota + 1
	KindImage
)

type Format uint8

const (
	FormatText Format = iota + 1
	FormatDIBV5
	FormatDIB
)

type Snapshot struct {
	CaptureID    string
	CapturedAt   time.Time
	Kind         Kind
	Text         string
	DIB          []byte
	Image        image.Image
	SourceFormat string
}

type Limits struct {
	MaxTextBytes  int64
	MaxImageBytes int64
}

type Formats struct {
	HasText  bool
	HasDIBV5 bool
	HasDIB   bool
}

func ChooseFormat(formats Formats) (Kind, string, error) {
	if formats.HasText {
		return KindText, "CF_UNICODETEXT", nil
	}
	if formats.HasDIBV5 {
		return KindImage, "CF_DIBV5", nil
	}
	if formats.HasDIB {
		return KindImage, "CF_DIB", nil
	}
	return 0, "", ErrUnsupported
}

func ValidateSize(kind Kind, size int64, limits Limits) error {
	if size < 0 {
		return fmt.Errorf("validate capture size: negative size %d", size)
	}

	var max int64
	switch kind {
	case KindText:
		max = limits.MaxTextBytes
	case KindImage:
		max = limits.MaxImageBytes
	default:
		return fmt.Errorf("%w: kind %d", ErrUnsupported, kind)
	}
	if max <= 0 {
		return fmt.Errorf("validate capture size: invalid limit %d for kind %d", max, kind)
	}
	if size > max {
		return fmt.Errorf("%w: kind %d size %d exceeds limit %d", ErrTooLarge, kind, size, max)
	}
	return nil
}

func NewCaptureID(at time.Time, random io.Reader) (string, error) {
	if random == nil {
		return "", errors.New("create capture ID: random source is nil")
	}
	var suffix [4]byte
	if _, err := io.ReadFull(random, suffix[:]); err != nil {
		return "", fmt.Errorf("create capture ID: read random suffix: %w", err)
	}
	return at.Format("20060102T150405") + "-" + hex.EncodeToString(suffix[:]), nil
}
