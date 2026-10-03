package main

import (
	"sync"
	"time"
)

// VirtualClock keeps an in-process offset only; the system clock is
// never touched.
type VirtualClock struct {
	mu      sync.RWMutex
	offset  time.Duration
	synced  bool
	source  string // name of upstream currently providing time
	stratum uint8  // stratum of selected upstream
	since   time.Time
}

func (c *VirtualClock) Set(offset time.Duration, sourceName string, stratum uint8) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset = offset
	c.synced = true
	c.source = sourceName
	c.stratum = stratum
	c.since = time.Now()
}

// Unset drops synchronization; old offsets are never reused.
func (c *VirtualClock) Unset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset = 0
	c.synced = false
	c.source = ""
	c.stratum = 0
}

func (c *VirtualClock) State() (synced bool, offset time.Duration, source string, stratum uint8) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.synced, c.offset, c.source, c.stratum
}

// Now returns local time plus the selected offset (or plain local
// time when unsynchronized).
func (c *VirtualClock) Now() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Now().Add(c.offset)
}
