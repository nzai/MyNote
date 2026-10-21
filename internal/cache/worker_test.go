package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nzai/MyNote/internal/core"
)

func TestWorkerProcessesJobsInOrder(t *testing.T) {
	store := &fakeWorkerStore{}
	var mu sync.Mutex
	var ids []string
	worker := NewWorker(store, func(event Event) {
		mu.Lock()
		defer mu.Unlock()
		ids = append(ids, event.ID)
	})

	worker.Enqueue(context.Background(), workerSnapshot("a"))
	worker.Enqueue(context.Background(), workerSnapshot("b"))
	requireEventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(ids) == 2
	})

	mu.Lock()
	got := append([]string(nil), ids...)
	mu.Unlock()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("event order = %v", got)
	}
	if err := worker.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerCancelBeforeProcessingSkipsSave(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	store := &fakeWorkerStore{saveFunc: func(ctx context.Context, snapshot core.Snapshot) (Record, error) {
		if snapshot.CaptureID == "a" {
			close(firstStarted)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return Record{}, ctx.Err()
			}
		}
		return Record{ID: snapshot.CaptureID}, nil
	}}
	events := make(chan Event, 4)
	worker := NewWorker(store, func(event Event) { events <- event })

	worker.Enqueue(context.Background(), workerSnapshot("a"))
	<-firstStarted
	cancelB := worker.Enqueue(context.Background(), workerSnapshot("b"))
	cancelB()
	close(releaseFirst)

	first := <-events
	second := <-events
	if first.ID != "a" || first.Err != nil {
		t.Fatalf("first event = %#v", first)
	}
	if second.ID != "b" || !second.Canceled {
		t.Fatalf("second event = %#v", second)
	}
	store.mu.Lock()
	saved := append([]string(nil), store.saved...)
	store.mu.Unlock()
	if len(saved) != 1 || saved[0] != "a" {
		t.Fatalf("saved IDs = %v", saved)
	}
	if err := worker.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerCancelDuringSaveReachesStoreContext(t *testing.T) {
	started := make(chan struct{})
	store := &fakeWorkerStore{saveFunc: func(ctx context.Context, _ core.Snapshot) (Record, error) {
		close(started)
		<-ctx.Done()
		return Record{}, ctx.Err()
	}}
	events := make(chan Event, 1)
	worker := NewWorker(store, func(event Event) { events <- event })

	cancel := worker.Enqueue(context.Background(), workerSnapshot("a"))
	<-started
	cancel()
	event := <-events
	if !event.Canceled || !errors.Is(event.Err, context.Canceled) {
		t.Fatalf("event = %#v", event)
	}
	if event.StartedAt.IsZero() || event.FinishedAt.Before(event.StartedAt) {
		t.Fatalf("timestamps = %s..%s", event.StartedAt, event.FinishedAt)
	}
	if err := worker.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerCloseWaitsForInFlightAndCancelsQueued(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	store := &fakeWorkerStore{saveFunc: func(ctx context.Context, snapshot core.Snapshot) (Record, error) {
		if snapshot.CaptureID == "a" {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return Record{}, ctx.Err()
			}
		}
		return Record{ID: snapshot.CaptureID}, nil
	}}
	events := make(chan Event, 4)
	worker := NewWorker(store, func(event Event) { events <- event })

	worker.Enqueue(context.Background(), workerSnapshot("a"))
	worker.Enqueue(context.Background(), workerSnapshot("b"))
	worker.Enqueue(context.Background(), workerSnapshot("c"))
	<-started

	closed := make(chan error, 1)
	go func() { closed <- worker.Close(context.Background()) }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned before in-flight save completed: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}

	got := []Event{<-events, <-events, <-events}
	if got[0].ID != "a" || got[0].Canceled {
		t.Fatalf("first event = %#v", got[0])
	}
	if got[1].ID != "b" || !got[1].Canceled || got[2].ID != "c" || !got[2].Canceled {
		t.Fatalf("queued events = %#v", got[1:])
	}
	select {
	case extra := <-events:
		t.Fatalf("unexpected extra event: %#v", extra)
	default:
	}
}

func TestWorkerCloseDeadlineCancelsInFlightSave(t *testing.T) {
	started := make(chan struct{})
	store := &fakeWorkerStore{saveFunc: func(ctx context.Context, _ core.Snapshot) (Record, error) {
		close(started)
		<-ctx.Done()
		return Record{}, ctx.Err()
	}}
	events := make(chan Event, 1)
	worker := NewWorker(store, func(event Event) { events <- event })
	worker.Enqueue(context.Background(), workerSnapshot("a"))
	<-started

	closeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := worker.Close(closeCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error = %v", err)
	}
	event := <-events
	if !event.Canceled {
		t.Fatalf("event = %#v", event)
	}
	if err := worker.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type fakeWorkerStore struct {
	mu       sync.Mutex
	saved    []string
	enforced int
	saveFunc func(context.Context, core.Snapshot) (Record, error)
}

func (s *fakeWorkerStore) Save(ctx context.Context, snapshot core.Snapshot) (Record, error) {
	s.mu.Lock()
	s.saved = append(s.saved, snapshot.CaptureID)
	s.mu.Unlock()
	if s.saveFunc != nil {
		return s.saveFunc(ctx, snapshot)
	}
	time.Sleep(time.Millisecond)
	return Record{ID: snapshot.CaptureID}, nil
}

func (s *fakeWorkerStore) Enforce(context.Context) (CleanupResult, error) {
	s.mu.Lock()
	s.enforced++
	s.mu.Unlock()
	return CleanupResult{}, nil
}

func workerSnapshot(id string) core.Snapshot {
	return core.Snapshot{
		CaptureID:    id,
		Kind:         core.KindText,
		Text:         id,
		CapturedAt:   fixedClock(),
		SourceFormat: "CF_UNICODETEXT",
	}
}

func requireEventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}
