package clipboard

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/nzai/MyNote/internal/core"
)

func TestReaderCapturesUnicodeText(t *testing.T) {
	session := &fakeSession{
		data: map[core.Format][]byte{
			core.FormatText: encodeUTF16("你好, clipboard"),
		},
	}
	reader := NewReader(
		&fakeOpener{sessions: []Session{session}},
		core.Limits{MaxTextBytes: 1024, MaxImageBytes: 1024},
		fixedReaderClock,
		bytes.NewReader([]byte{1, 2, 3, 4}),
	)

	got, err := reader.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != core.KindText || got.Text != "你好, clipboard" {
		t.Fatalf("unexpected snapshot %#v", got)
	}
	if got.SourceFormat != "CF_UNICODETEXT" {
		t.Fatalf("source format = %q", got.SourceFormat)
	}
	if got.CaptureID != "20261021T153012-01020304" {
		t.Fatalf("capture ID = %q", got.CaptureID)
	}
	if !session.closed {
		t.Fatal("session was not closed")
	}
}

func TestReaderReturnsUnsupportedForEmptyClipboard(t *testing.T) {
	reader := testReader(&fakeSession{})
	_, err := reader.Read(context.Background())
	if !errors.Is(err, core.ErrUnsupported) {
		t.Fatalf("got %v, want ErrUnsupported", err)
	}
}

func TestReaderRejectsOversizedText(t *testing.T) {
	reader := testReader(&fakeSession{
		data: map[core.Format][]byte{
			core.FormatText: encodeUTF16(strings.Repeat("a", 33)),
		},
	})
	reader.limits.MaxTextBytes = 32

	_, err := reader.Read(context.Background())
	if !errors.Is(err, core.ErrTooLarge) {
		t.Fatalf("got %v, want ErrTooLarge", err)
	}
}

func TestReaderFallsBackFromEmptyTextToDIB(t *testing.T) {
	dib := makeDIB(dibSpec{
		width:    1,
		height:   1,
		bitCount: 24,
		rows:     [][]byte{{0, 0, 255, 0}},
	})
	reader := testReader(&fakeSession{
		data: map[core.Format][]byte{
			core.FormatText: encodeUTF16(""),
			core.FormatDIB:  dib,
		},
	})

	got, err := reader.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != core.KindImage || got.SourceFormat != "CF_DIB" || got.Image == nil {
		t.Fatalf("unexpected snapshot: kind=%v source=%q image=%v", got.Kind, got.SourceFormat, got.Image)
	}
}

func TestReaderFallsBackFromInvalidDIBV5ToDIB(t *testing.T) {
	dib := makeDIB(dibSpec{
		width:    1,
		height:   1,
		bitCount: 24,
		rows:     [][]byte{{0, 0, 255, 0}},
	})
	reader := testReader(&fakeSession{
		data: map[core.Format][]byte{
			core.FormatDIBV5: []byte{1, 2, 3},
			core.FormatDIB:   dib,
		},
	})

	got, err := reader.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceFormat != "CF_DIB" {
		t.Fatalf("source format = %q", got.SourceFormat)
	}
}

func TestReaderDoesNotConvertInvalidDIBToText(t *testing.T) {
	reader := testReader(&fakeSession{
		data: map[core.Format][]byte{
			core.FormatDIB: []byte{1, 2, 3},
		},
	})

	_, err := reader.Read(context.Background())
	if err == nil || errors.Is(err, core.ErrUnsupported) {
		t.Fatalf("expected DIB decode error, got %v", err)
	}
}

func TestReaderRetriesOpenAndHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	opener := &fakeOpener{errs: []error{errors.New("clipboard busy")}}
	reader := NewReader(opener, testLimits, fixedReaderClock, bytes.NewReader(make([]byte, 4)))
	reader.sleep = func(context.Context, time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, err := reader.Read(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if opener.calls != 1 {
		t.Fatalf("open calls = %d, want 1", opener.calls)
	}
}

func TestReaderRetriesWithBackoff(t *testing.T) {
	opener := &fakeOpener{
		errs: []error{errors.New("busy"), errors.New("busy")},
		sessions: []Session{&fakeSession{
			data: map[core.Format][]byte{core.FormatText: encodeUTF16("ready")},
		}},
	}
	reader := NewReader(opener, testLimits, fixedReaderClock, bytes.NewReader(make([]byte, 4)))
	var delays []time.Duration
	reader.sleep = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}

	got, err := reader.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "ready" {
		t.Fatalf("text = %q", got.Text)
	}
	want := []time.Duration{20 * time.Millisecond, 40 * time.Millisecond}
	if len(delays) != len(want) || delays[0] != want[0] || delays[1] != want[1] {
		t.Fatalf("delays = %v, want %v", delays, want)
	}
}

func TestReaderJoinsCloseFailureWithSuccessfulCapture(t *testing.T) {
	closeErr := errors.New("close failed")
	session := &fakeSession{
		data:     map[core.Format][]byte{core.FormatText: encodeUTF16("hello")},
		closeErr: closeErr,
	}
	reader := testReader(session)

	got, err := reader.Read(context.Background())
	if !errors.Is(err, closeErr) {
		t.Fatalf("got %v, want close error", err)
	}
	if got.Text != "hello" {
		t.Fatalf("successful capture should be preserved, got %#v", got)
	}
}

func TestDecodeUTF16TruncatesAtFirstNUL(t *testing.T) {
	data := append(encodeUTF16("before"), encodeUTF16("after")...)
	got, err := decodeUTF16LE(data)
	if err != nil {
		t.Fatal(err)
	}
	if got != "before" {
		t.Fatalf("text = %q", got)
	}
}

func TestDecodeUTF16RejectsOddLength(t *testing.T) {
	if _, err := decodeUTF16LE([]byte{1}); err == nil {
		t.Fatal("expected odd length error")
	}
}

func testReader(session Session) *Reader {
	return NewReader(
		&fakeOpener{sessions: []Session{session}},
		testLimits,
		fixedReaderClock,
		bytes.NewReader([]byte{1, 2, 3, 4}),
	)
}

var testLimits = core.Limits{MaxTextBytes: 1024, MaxImageBytes: 1024}

func fixedReaderClock() time.Time {
	return time.Date(2026, 10, 21, 15, 30, 12, 0, time.Local)
}

type fakeOpener struct {
	sessions []Session
	errs     []error
	calls    int
}

func (o *fakeOpener) Open() (Session, error) {
	index := o.calls
	o.calls++
	if index < len(o.errs) {
		return nil, o.errs[index]
	}
	index -= len(o.errs)
	if index >= 0 && index < len(o.sessions) {
		return o.sessions[index], nil
	}
	return nil, errors.New("no fake session")
}

type fakeSession struct {
	data     map[core.Format][]byte
	readErr  map[core.Format]error
	closeErr error
	closed   bool
}

func (s *fakeSession) Available(format core.Format) bool {
	_, ok := s.data[format]
	return ok
}

func (s *fakeSession) Read(format core.Format) ([]byte, error) {
	if err := s.readErr[format]; err != nil {
		return nil, err
	}
	data, ok := s.data[format]
	if !ok {
		return nil, errors.New("format unavailable")
	}
	return append([]byte(nil), data...), nil
}

func (s *fakeSession) Close() error {
	s.closed = true
	return s.closeErr
}

func encodeUTF16(text string) []byte {
	units := utf16.Encode([]rune(text))
	data := make([]byte, (len(units)+1)*2)
	for index, unit := range units {
		data[index*2] = byte(unit)
		data[index*2+1] = byte(unit >> 8)
	}
	return data
}
