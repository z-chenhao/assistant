package memory

import (
	"context"
	"time"
)

// Curator owns the serial maintenance lifecycle and internal retry policy.
// Process provides one bounded, durable batch; shutdown always joins this loop.
type Curator struct {
	Process  func(context.Context) error
	Deferred func()
}

func (c Curator) Run(ctx context.Context) {
	delay := 250 * time.Millisecond
	for ctx.Err() == nil {
		if e := c.Process(ctx); e != nil {
			if ctx.Err() != nil {
				return
			}
			if c.Deferred != nil {
				c.Deferred()
			}
			delay = min(30*time.Second, max(time.Second, delay*2))
		} else {
			delay = 250 * time.Millisecond
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
