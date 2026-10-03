package ntp

import (
	"sync"
	"time"
)

// Clock is a process-local virtual clock: local monotonic time plus an
// in-memory offset. It never touches the system clock.
type Clock struct {
	mu     sync.RWMutex
	offset time.Duration
	synced bool
	// Stratum of the currently selected upstream, valid when synced.
	Stratum uint8
}

// Update applies a selection. When the selection is not synchronized the
// old offset is discarded immediately (no stale offset reuse).
func (c *Clock) Update(sel Selection, stratum uint8) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if sel.Synchronized {
		c.offset = sel.Offset
		c.synced = true
		c.Stratum = stratum
	} else {
		c.offset = 0
		c.synced = false
		c.Stratum = 0
	}
}

func (c *Clock) Synchronized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.synced
}

func (c *Clock) Offset() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.offset
}

// Now returns virtual time (local time plus the selected offset).
func (c *Clock) Now() time.Time {
	return time.Now().Add(c.Offset())
}
