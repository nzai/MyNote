package ui

import (
	"testing"
	"time"
)

func TestProgressHiddenBeforeThreshold(t *testing.T) {
	p := newProgressState(3 * time.Second)
	p.Start(time.Unix(0, 0))

	if p.Visible(time.Unix(2, 0)) {
		t.Fatal("progress must stay hidden before 3s")
	}
	if !p.Visible(time.Unix(4, 0)) {
		t.Fatal("progress must appear after 3s")
	}
}

func TestProgressStaysHiddenAfterCompletion(t *testing.T) {
	p := newProgressState(3 * time.Second)
	p.Start(time.Unix(0, 0))
	p.Finish()

	if p.Visible(time.Unix(4, 0)) {
		t.Fatal("completed progress must not become visible later")
	}
}
