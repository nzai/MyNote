package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nzai/MyNote/internal/core"
)

var ErrWorkerClosed = errors.New("cache worker is closed")

type WorkerStore interface {
	Save(context.Context, core.Snapshot) (Record, error)
	Enforce(context.Context) (CleanupResult, error)
}

type Event struct {
	ID         string
	Record     Record
	Err        error
	Canceled   bool
	StartedAt  time.Time
	FinishedAt time.Time
}

type Worker struct {
	store    WorkerStore
	callback func(Event)
	now      func() time.Time

	mu           sync.Mutex
	queue        []*workerJob
	closed       bool
	activeCancel context.CancelFunc
	wake         chan struct{}
	done         chan struct{}
}

type workerJob struct {
	snapshot core.Snapshot
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewWorker(store WorkerStore, callback func(Event)) *Worker {
	worker := &Worker{
		store:    store,
		callback: callback,
		now:      time.Now,
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
	}
	go worker.run()
	return worker
}

func (w *Worker) Enqueue(ctx context.Context, snapshot core.Snapshot) func() {
	if ctx == nil {
		ctx = context.Background()
	}
	jobCtx, cancel := context.WithCancel(ctx)
	job := &workerJob{snapshot: snapshot, ctx: jobCtx, cancel: cancel}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		cancel()
		return func() {}
	}
	w.queue = append(w.queue, job)
	w.mu.Unlock()
	w.signal()
	return cancel
}

func (w *Worker) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	w.mu.Lock()
	if !w.closed {
		w.closed = true
		for _, job := range w.queue {
			job.cancel()
		}
	}
	w.mu.Unlock()
	w.signal()

	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		w.mu.Lock()
		if w.activeCancel != nil {
			w.activeCancel()
		}
		for _, job := range w.queue {
			job.cancel()
		}
		w.mu.Unlock()
		return fmt.Errorf("close cache worker: %w", ctx.Err())
	}
}

func (w *Worker) run() {
	defer close(w.done)
	for {
		job, ok := w.next()
		if !ok {
			return
		}
		w.process(job)
		job.cancel()
	}
}

func (w *Worker) next() (*workerJob, bool) {
	for {
		w.mu.Lock()
		if len(w.queue) > 0 {
			job := w.queue[0]
			w.queue = w.queue[1:]
			w.activeCancel = job.cancel
			w.mu.Unlock()
			return job, true
		}
		if w.closed {
			w.mu.Unlock()
			return nil, false
		}
		w.mu.Unlock()

		<-w.wake
	}
}

func (w *Worker) process(job *workerJob) {
	started := w.now()
	var record Record
	var err error
	if ctxErr := job.ctx.Err(); ctxErr != nil {
		err = ctxErr
	} else if w.store == nil {
		err = errors.New("cache worker store is nil")
	} else {
		record, err = w.store.Save(job.ctx, job.snapshot)
		if err == nil {
			if _, cleanupErr := w.store.Enforce(job.ctx); cleanupErr != nil && !isCanceled(job.ctx, cleanupErr) {
				err = fmt.Errorf("enforce cache retention after save: %w", cleanupErr)
			}
		}
	}

	event := Event{
		ID:         job.snapshot.CaptureID,
		Record:     record,
		Err:        err,
		Canceled:   isCanceled(job.ctx, err),
		StartedAt:  started,
		FinishedAt: w.now(),
	}
	w.mu.Lock()
	w.activeCancel = nil
	w.mu.Unlock()
	if w.callback != nil {
		w.callback(event)
	}
}

func (w *Worker) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func isCanceled(ctx context.Context, err error) bool {
	return err != nil &&
		(errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(ctx.Err(), context.Canceled) ||
			errors.Is(ctx.Err(), context.DeadlineExceeded))
}
