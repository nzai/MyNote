package ui

import "time"

type progressState struct {
	delay   time.Duration
	started time.Time
	active  bool
}

func newProgressState(delay time.Duration) progressState {
	if delay < 0 {
		delay = 0
	}
	return progressState{delay: delay}
}

func (p *progressState) Start(at time.Time) {
	p.started = at
	p.active = true
}

func (p *progressState) Visible(at time.Time) bool {
	if !p.active || p.started.IsZero() || at.Before(p.started) {
		return false
	}
	return at.Sub(p.started) >= p.delay
}

func (p *progressState) Finish() {
	p.active = false
}
