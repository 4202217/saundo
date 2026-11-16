package sender

import "time"

// timer is a one-shot or repeating deadline driven by Tick. The sender owns no
// timer of its own; the host advances this state.
type timer struct {
	interval  time.Duration
	repeating bool
	active    bool
	due       time.Time
}

func (t *timer) arm(now time.Time) {
	t.due = now.Add(t.interval)
	t.active = true
}

func (t *timer) disarm() { t.active = false }
