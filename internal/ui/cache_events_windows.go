//go:build windows

package ui

import (
	"errors"
	"fmt"
	"sync"

	"github.com/nzai/MyNote/internal/cache"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const cacheEventMessage = co.WM_APP + 102

var ErrCacheEventSinkClosed = errors.New("cache event sink is closed")

type CacheEventSink struct {
	mu     sync.Mutex
	hwnd   win.HWND
	events []cache.Event
	closed bool
}

func NewCacheEventSink() *CacheEventSink {
	return &CacheEventSink{}
}

func (s *CacheEventSink) Bind(hwnd win.HWND) error {
	if s == nil {
		return errors.New("bind cache event sink: sink is nil")
	}
	if hwnd == 0 {
		return errors.New("bind cache event sink: hwnd is null")
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrCacheEventSinkClosed
	}
	s.hwnd = hwnd
	pending := len(s.events) > 0
	s.mu.Unlock()

	if pending {
		if err := hwnd.PostMessage(cacheEventMessage, 0, 0); err != nil {
			return fmt.Errorf("post pending cache event message: %w", err)
		}
	}
	return nil
}

func (s *CacheEventSink) Push(event cache.Event) error {
	if s == nil {
		return errors.New("push cache event: sink is nil")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrCacheEventSinkClosed
	}
	s.events = append(s.events, event)
	hwnd := s.hwnd
	s.mu.Unlock()

	if hwnd == 0 {
		return nil
	}
	if err := hwnd.PostMessage(cacheEventMessage, 0, 0); err != nil {
		return fmt.Errorf("post cache event message for %q: %w", event.ID, err)
	}
	return nil
}

func (s *CacheEventSink) Drain() []cache.Event {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.events) == 0 {
		return nil
	}
	events := s.events
	s.events = nil
	return events
}

func (s *CacheEventSink) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.hwnd = 0
	s.events = nil
}
