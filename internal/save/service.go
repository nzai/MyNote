package save

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nzai/MyNote/internal/core"
)

var ErrCanceled = errors.New("save canceled")

type DialogOptions struct {
	Parent        uintptr
	Kind          core.Kind
	SuggestedName string
	Title         string
}

type Dialog interface {
	Choose(context.Context, DialogOptions) (string, error)
}

type Writer interface {
	Write(context.Context, string, core.Snapshot, string) error
}

type Prepared struct {
	snapshot core.Snapshot
	state    *core.SaveState
	path     string
	text     string
}

func (p Prepared) Path() string {
	return p.path
}

type Service struct {
	mu     sync.Mutex
	dialog Dialog
	writer Writer
	now    func() time.Time

	snapshot core.Snapshot
	state    *core.SaveState
}

func NewService(dialog Dialog, writer Writer, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		dialog: dialog,
		writer: writer,
		now:    now,
		snapshot: core.Snapshot{
			Kind: core.KindText,
		},
		state: core.NewSaveState(""),
	}
}

func (s *Service) SetSnapshot(snapshot core.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.snapshot = snapshot
	switch snapshot.Kind {
	case core.KindText:
		s.state = core.NewSaveState(snapshot.Text)
	case core.KindImage:
		s.state = core.NewReadOnlySaveState()
	default:
		s.state = core.NewReadOnlySaveState()
	}
}

func (s *Service) SetText(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.SetText(text)
}

func (s *Service) Text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Text()
}

func (s *Service) Kind() core.Kind {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot.Kind
}

func (s *Service) Dirty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Dirty()
}

func (s *Service) Path() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Path()
}

func (s *Service) Snapshot() core.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}

func (s *Service) Save(ctx context.Context, parent uintptr) (string, error) {
	return s.save(ctx, parent, false)
}

func (s *Service) SaveAs(ctx context.Context, parent uintptr) (string, error) {
	return s.save(ctx, parent, true)
}

func (s *Service) MarkSaved(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.MarkSaved(path)
}

func (s *Service) save(ctx context.Context, parent uintptr, forceDialog bool) (string, error) {
	prepared, err := s.Prepare(ctx, parent, forceDialog)
	if err != nil {
		return "", err
	}
	if err := s.WritePrepared(ctx, prepared); err != nil {
		return "", err
	}
	s.CommitPrepared(prepared)
	return prepared.path, nil
}

func (s *Service) Prepare(
	ctx context.Context,
	parent uintptr,
	forceDialog bool,
) (Prepared, error) {
	if ctx == nil {
		return Prepared{}, errors.New("save service: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Prepared{}, fmt.Errorf("save service: %w", err)
	}

	s.mu.Lock()
	snapshot := s.snapshot
	state := s.state
	text := state.Text()
	path := state.Path()
	s.mu.Unlock()

	if forceDialog || path == "" {
		if s.dialog == nil {
			return Prepared{}, errors.New("save service: dialog is nil")
		}
		chosen, err := s.dialog.Choose(ctx, DialogOptions{
			Parent:        parent,
			Kind:          snapshot.Kind,
			SuggestedName: suggestedName(snapshot.Kind, s.now()),
			Title:         "另存为",
		})
		if err != nil {
			if errors.Is(err, ErrCanceled) {
				return Prepared{}, ErrCanceled
			}
			return Prepared{}, fmt.Errorf("choose save path for kind %d: %w", snapshot.Kind, err)
		}
		if chosen == "" {
			return Prepared{}, ErrCanceled
		}
		path = chosen
	}
	if s.writer == nil {
		return Prepared{}, errors.New("save service: writer is nil")
	}
	return Prepared{
		snapshot: snapshot,
		state:    state,
		path:     path,
		text:     text,
	}, nil
}

func (s *Service) WritePrepared(ctx context.Context, prepared Prepared) error {
	if ctx == nil {
		return errors.New("save service: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("save service: %w", err)
	}
	if s.writer == nil {
		return errors.New("save service: writer is nil")
	}
	if prepared.path == "" {
		return errors.New("save service: prepared path is empty")
	}
	if err := s.writer.Write(ctx, prepared.path, prepared.snapshot, prepared.text); err != nil {
		return fmt.Errorf("write saved document %q: %w", prepared.path, err)
	}
	return nil
}

func (s *Service) CommitPrepared(prepared Prepared) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == prepared.state {
		s.state.MarkSavedText(prepared.path, prepared.text)
	}
}

func suggestedName(kind core.Kind, at time.Time) string {
	stamp := at.Format("20060102_150405")
	switch kind {
	case core.KindImage:
		return "剪贴板图片_" + stamp + ".png"
	default:
		return "剪贴板文本_" + stamp + ".txt"
	}
}
