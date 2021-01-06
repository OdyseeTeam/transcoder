package timer

import (
	"fmt"
	"time"
)

type Timer struct {
	Started  time.Time
	duration float64
}

func Start() *Timer {
	return &Timer{Started: time.Now()}
}

func (t *Timer) Stop() float64 {
	if t.duration == 0 {
		t.duration = time.Since(t.Started).Seconds()
	}
	return t.duration
}

func (t *Timer) Duration() float64 {
	if t.duration == 0 {
		return time.Since(t.Started).Seconds()
	}
	return t.duration
}

func (t *Timer) DurationInt() int64 {
	if t.duration == 0 {
		return int64(time.Since(t.Started).Seconds())
	}
	return int64(t.duration)
}

func (t *Timer) String() string {
	return fmt.Sprintf("%.2f", t.Duration())
}
