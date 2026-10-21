//go:build windows

package ui

import (
	"testing"

	"github.com/nzai/MyNote/internal/cache"
)

func TestCacheEventSinkQueuesBeforeBinding(t *testing.T) {
	sink := NewCacheEventSink()
	if err := sink.Push(cache.Event{ID: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Push(cache.Event{ID: "second"}); err != nil {
		t.Fatal(err)
	}

	events := sink.Drain()
	if len(events) != 2 || events[0].ID != "first" || events[1].ID != "second" {
		t.Fatalf("drained events = %#v", events)
	}
	if extra := sink.Drain(); len(extra) != 0 {
		t.Fatalf("expected empty second drain, got %#v", extra)
	}
}

func TestCacheEventSinkRejectsPushAfterClose(t *testing.T) {
	sink := NewCacheEventSink()
	sink.Close()
	if err := sink.Push(cache.Event{ID: "late"}); err == nil {
		t.Fatal("expected push after close to fail")
	}
}
