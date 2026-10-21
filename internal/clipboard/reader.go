package clipboard

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"
	"unicode/utf16"

	"github.com/nzai/MyNote/internal/core"
)

var retryDelays = []time.Duration{
	20 * time.Millisecond,
	40 * time.Millisecond,
	80 * time.Millisecond,
	160 * time.Millisecond,
	320 * time.Millisecond,
}

type Session interface {
	Available(core.Format) bool
	HasAnyFormat() bool
	Read(core.Format) ([]byte, error)
	Close() error
}

type Opener interface {
	Open() (Session, error)
}

type Reader struct {
	opener Opener
	limits core.Limits
	now    func() time.Time
	random io.Reader
	sleep  func(context.Context, time.Duration) error
}

func NewReader(
	opener Opener,
	limits core.Limits,
	now func() time.Time,
	random io.Reader,
) *Reader {
	if now == nil {
		now = time.Now
	}
	if random == nil {
		random = rand.Reader
	}
	return &Reader{
		opener: opener,
		limits: limits,
		now:    now,
		random: random,
		sleep:  sleepContext,
	}
}

func (r *Reader) Read(ctx context.Context) (core.Snapshot, error) {
	if ctx == nil {
		return core.Snapshot{}, errors.New("read clipboard: context is nil")
	}
	if r.opener == nil {
		return core.Snapshot{}, errors.New("read clipboard: opener is nil")
	}

	session, err := r.open(ctx)
	if err != nil {
		return core.Snapshot{}, err
	}
	snapshot, captureErr := r.capture(ctx, session)
	closeErr := session.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close clipboard session: %w", closeErr)
	}

	switch {
	case captureErr == nil && closeErr == nil:
		return snapshot, nil
	case captureErr == nil:
		return snapshot, closeErr
	case closeErr == nil:
		return core.Snapshot{}, captureErr
	default:
		return core.Snapshot{}, errors.Join(captureErr, closeErr)
	}
}

func (r *Reader) open(ctx context.Context) (Session, error) {
	var lastErr error
	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("open clipboard: %w", err)
		}
		session, err := r.opener.Open()
		if err == nil {
			return session, nil
		}
		lastErr = err
		if attempt == len(retryDelays) {
			break
		}
		if err := r.sleep(ctx, retryDelays[attempt]); err != nil {
			return nil, fmt.Errorf("wait to retry clipboard open: %w", err)
		}
	}
	return nil, fmt.Errorf("open clipboard after %d attempts: %w", len(retryDelays)+1, lastErr)
}

func (r *Reader) capture(ctx context.Context, session Session) (core.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return core.Snapshot{}, fmt.Errorf("capture clipboard: %w", err)
	}

	formats := core.Formats{
		HasText:  session.Available(core.FormatText),
		HasDIBV5: session.Available(core.FormatDIBV5),
		HasDIB:   session.Available(core.FormatDIB),
	}
	if !formats.HasText && !formats.HasDIBV5 && !formats.HasDIB && !session.HasAnyFormat() {
		return r.finishSnapshot(core.Snapshot{
			Kind: core.KindText,
			Text: "",
		})
	}
	kind, source, err := core.ChooseFormat(formats)
	if err != nil {
		return core.Snapshot{}, fmt.Errorf("capture clipboard format: %w", err)
	}

	if kind == core.KindText {
		snapshot, empty, err := r.captureText(session, source)
		if err != nil {
			return core.Snapshot{}, err
		}
		if !empty {
			return r.finishSnapshot(snapshot)
		}
		snapshot, err = r.captureAvailableImage(session, formats)
		if err != nil {
			return core.Snapshot{}, err
		}
		return r.finishSnapshot(snapshot)
	}

	snapshot, err := r.captureAvailableImage(session, formats)
	if err != nil {
		return core.Snapshot{}, err
	}
	return r.finishSnapshot(snapshot)
}

func (r *Reader) captureAvailableImage(
	session Session,
	formats core.Formats,
) (core.Snapshot, error) {
	var failures []error
	if formats.HasDIBV5 {
		snapshot, err := r.captureImage(session, core.FormatDIBV5, "CF_DIBV5")
		if err == nil {
			return snapshot, nil
		}
		failures = append(failures, err)
	}
	if formats.HasDIB {
		snapshot, err := r.captureImage(session, core.FormatDIB, "CF_DIB")
		if err == nil {
			return snapshot, nil
		}
		failures = append(failures, err)
	}
	if len(failures) == 0 {
		return core.Snapshot{}, fmt.Errorf("capture clipboard image: %w", core.ErrUnsupported)
	}
	if len(failures) == 1 {
		return core.Snapshot{}, failures[0]
	}
	return core.Snapshot{}, errors.Join(failures...)
}

func (r *Reader) captureText(session Session, source string) (core.Snapshot, bool, error) {
	data, err := session.Read(core.FormatText)
	if err != nil {
		return core.Snapshot{}, false, fmt.Errorf("read CF_UNICODETEXT: %w", err)
	}
	text, err := decodeUTF16LE(data)
	if err != nil {
		return core.Snapshot{}, false, fmt.Errorf("decode CF_UNICODETEXT: %w", err)
	}
	if text == "" {
		return core.Snapshot{}, true, nil
	}
	if err := core.ValidateSize(core.KindText, int64(len(text)), r.limits); err != nil {
		return core.Snapshot{}, false, fmt.Errorf("validate clipboard text: %w", err)
	}
	return core.Snapshot{
		Kind:         core.KindText,
		Text:         text,
		SourceFormat: source,
	}, false, nil
}

func (r *Reader) captureImage(
	session Session,
	format core.Format,
	source string,
) (core.Snapshot, error) {
	data, err := session.Read(format)
	if err != nil {
		return core.Snapshot{}, fmt.Errorf("read %s: %w", source, err)
	}
	if err := core.ValidateSize(core.KindImage, int64(len(data)), r.limits); err != nil {
		return core.Snapshot{}, fmt.Errorf("validate clipboard image: %w", err)
	}
	img, err := DecodeDIB(data)
	if err != nil {
		return core.Snapshot{}, fmt.Errorf("decode %s image: %w", source, err)
	}
	return core.Snapshot{
		Kind:         core.KindImage,
		DIB:          data,
		Image:        img,
		SourceFormat: source,
	}, nil
}

func (r *Reader) finishSnapshot(snapshot core.Snapshot) (core.Snapshot, error) {
	at := r.now()
	id, err := core.NewCaptureID(at, r.random)
	if err != nil {
		return core.Snapshot{}, fmt.Errorf("assign clipboard capture ID: %w", err)
	}
	snapshot.CapturedAt = at
	snapshot.CaptureID = id
	return snapshot, nil
}

func decodeUTF16LE(data []byte) (string, error) {
	if len(data)%2 != 0 {
		return "", fmt.Errorf("UTF-16LE data has odd byte length %d", len(data))
	}
	units := make([]uint16, 0, len(data)/2)
	for offset := 0; offset < len(data); offset += 2 {
		unit := uint16(data[offset]) | uint16(data[offset+1])<<8
		if unit == 0 {
			break
		}
		units = append(units, unit)
	}
	return string(utf16.Decode(units)), nil
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
