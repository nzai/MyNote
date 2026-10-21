package save

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nzai/MyNote/internal/core"
)

func TestServiceFirstSaveUsesDialogAndMarksClean(t *testing.T) {
	dialog := &fakeDialog{path: `C:\Temp\clip.txt`}
	writer := &fakeWriter{}
	service := NewService(dialog, writer, fixedSaveClock)
	service.SetText("hello")

	got, err := service.Save(context.Background(), 123)
	if err != nil {
		t.Fatal(err)
	}
	if got != dialog.path || service.Dirty() {
		t.Fatalf("path=%q dirty=%v", got, service.Dirty())
	}
	if writer.calls != 1 {
		t.Fatalf("writer calls=%d", writer.calls)
	}
	if dialog.calls != 1 {
		t.Fatalf("dialog calls=%d", dialog.calls)
	}
}

func TestServiceSubsequentSaveReusesCurrentPath(t *testing.T) {
	dialog := &fakeDialog{path: `C:\Temp\clip.txt`}
	writer := &fakeWriter{}
	service := NewService(dialog, writer, fixedSaveClock)
	service.SetText("first")

	if _, err := service.Save(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	service.SetText("second")
	path, err := service.Save(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if path != dialog.path || dialog.calls != 1 || writer.calls != 2 {
		t.Fatalf("path=%q dialog=%d writer=%d", path, dialog.calls, writer.calls)
	}
	if service.Dirty() {
		t.Fatal("successful second save must be clean")
	}
}

func TestServiceSaveAsAlwaysUsesDialogAndUpdatesPath(t *testing.T) {
	firstPath := `C:\Temp\first.txt`
	secondPath := `C:\Temp\second.txt`
	dialog := &fakeDialog{path: firstPath}
	writer := &fakeWriter{}
	service := NewService(dialog, writer, fixedSaveClock)
	service.SetText("hello")
	if _, err := service.Save(context.Background(), 1); err != nil {
		t.Fatal(err)
	}

	dialog.path = secondPath
	got, err := service.SaveAs(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != secondPath || service.Path() != secondPath {
		t.Fatalf("save-as path=%q current=%q", got, service.Path())
	}
	if dialog.calls != 2 || writer.calls != 2 {
		t.Fatalf("dialog=%d writer=%d", dialog.calls, writer.calls)
	}
}

func TestServiceDialogCancelPreservesPathAndDirtyState(t *testing.T) {
	dialog := &fakeDialog{path: `C:\Temp\first.txt`}
	writer := &fakeWriter{}
	service := NewService(dialog, writer, fixedSaveClock)
	service.SetText("saved")
	if _, err := service.Save(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	service.SetText("edited")
	dialog.err = ErrCanceled

	_, err := service.SaveAs(context.Background(), 1)
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("error = %v", err)
	}
	if service.Path() != `C:\Temp\first.txt` || !service.Dirty() {
		t.Fatalf("path=%q dirty=%v", service.Path(), service.Dirty())
	}
	if writer.calls != 1 {
		t.Fatalf("writer calls = %d", writer.calls)
	}
}

func TestServiceWriterFailurePreservesPathAndDirtyState(t *testing.T) {
	dialog := &fakeDialog{path: `C:\Temp\clip.txt`}
	writeErr := errors.New("disk full")
	writer := &fakeWriter{err: writeErr}
	service := NewService(dialog, writer, fixedSaveClock)
	service.SetText("hello")

	_, err := service.Save(context.Background(), 1)
	if !errors.Is(err, writeErr) {
		t.Fatalf("error = %v", err)
	}
	if service.Path() != "" || !service.Dirty() {
		t.Fatalf("path=%q dirty=%v", service.Path(), service.Dirty())
	}
}

func TestServiceSuggestedNamesMatchContentKind(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		dialog := &fakeDialog{path: `C:\Temp\clip.txt`}
		service := NewService(dialog, &fakeWriter{}, fixedSaveClock)
		service.SetText("hello")
		if _, err := service.Save(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
		if want := "剪贴板文本_20261021_153012.txt"; dialog.options.SuggestedName != want {
			t.Fatalf("suggested name = %q, want %q", dialog.options.SuggestedName, want)
		}
		if dialog.options.Kind != core.KindText || dialog.options.Parent != 1 {
			t.Fatalf("options = %#v", dialog.options)
		}
	})
	t.Run("image", func(t *testing.T) {
		dialog := &fakeDialog{path: `C:\Temp\clip.png`}
		service := NewService(dialog, &fakeWriter{}, fixedSaveClock)
		service.SetSnapshot(core.Snapshot{Kind: core.KindImage})
		if _, err := service.Save(context.Background(), 2); err != nil {
			t.Fatal(err)
		}
		if want := "剪贴板图片_20261021_153012.png"; dialog.options.SuggestedName != want {
			t.Fatalf("suggested name = %q, want %q", dialog.options.SuggestedName, want)
		}
		if dialog.options.Kind != core.KindImage {
			t.Fatalf("kind = %v", dialog.options.Kind)
		}
	})
}

func TestServiceKeepsNewerEditsDirtyWhenSaveCompletes(t *testing.T) {
	dialog := &fakeDialog{path: `C:\Temp\clip.txt`}
	started := make(chan struct{})
	release := make(chan struct{})
	writer := &fakeWriter{writeFunc: func(context.Context, string, core.Snapshot, string) error {
		close(started)
		<-release
		return nil
	}}
	service := NewService(dialog, writer, fixedSaveClock)
	service.SetText("first")

	done := make(chan error, 1)
	go func() {
		_, err := service.Save(context.Background(), 1)
		done <- err
	}()
	<-started
	service.SetText("newer")
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !service.Dirty() {
		t.Fatal("newer edit must remain dirty")
	}
	if service.Path() != dialog.path {
		t.Fatalf("path = %q", service.Path())
	}
	if strings.TrimSpace(writer.text) != "first" {
		t.Fatalf("written text = %q", writer.text)
	}
}

func TestPrepareDeferResWriteUntilRequested(t *testing.T) {
	dialog := &fakeDialog{path: `C:\Temp\prepared.txt`}
	writer := &fakeWriter{}
	service := NewService(dialog, writer, fixedSaveClock)
	service.SetText("prepared")

	prepared, err := service.Prepare(context.Background(), 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if writer.calls != 0 {
		t.Fatalf("writer calls after prepare = %d, want 0", writer.calls)
	}
	if prepared.Path() != dialog.path {
		t.Fatalf("prepared path = %q, want %q", prepared.Path(), dialog.path)
	}

	if err := service.WritePrepared(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	service.CommitPrepared(prepared)
	if writer.calls != 1 || service.Dirty() {
		t.Fatalf("writer calls = %d, dirty = %v", writer.calls, service.Dirty())
	}
}

type fakeDialog struct {
	path    string
	err     error
	calls   int
	options DialogOptions
}

func (d *fakeDialog) Choose(_ context.Context, options DialogOptions) (string, error) {
	d.calls++
	d.options = options
	return d.path, d.err
}

type fakeWriter struct {
	calls     int
	path      string
	kind      core.Kind
	text      string
	err       error
	writeFunc func(context.Context, string, core.Snapshot, string) error
}

func (w *fakeWriter) Write(
	ctx context.Context,
	path string,
	snapshot core.Snapshot,
	text string,
) error {
	w.calls++
	w.path = path
	w.kind = snapshot.Kind
	w.text = text
	if w.writeFunc != nil {
		return w.writeFunc(ctx, path, snapshot, text)
	}
	return w.err
}

func fixedSaveClock() time.Time {
	return time.Date(2026, 10, 21, 15, 30, 12, 0, time.Local)
}
