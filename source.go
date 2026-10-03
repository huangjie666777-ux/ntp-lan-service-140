package main

import (
	"sync"
	"time"
)

const maxSamples = 8

// Sample is one accepted four-timestamp measurement.
type Sample struct {
	Offset   time.Duration
	Delay    time.Duration
	RecvMono time.Time // monotonic receive time, for expiry
	RecvWall time.Time
}

// Source tracks the state of one upstream server.
type Source struct {
	Name  string
	Addr  string
	Order int // config order, for tie-breaking

	mu          sync.Mutex
	samples     []Sample // newest last, max 8
	lastReject  string
	lastPollErr string
	lastRespTx  NTPTime // transmit ts of last accepted response, dup detection
	lastStratum uint8   // stratum reported by the upstream
	polls       uint64
	accepted    uint64
	rejected    uint64
}

func (s *Source) StratumHint() uint8 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastStratum
}

func (s *Source) setStratum(st uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastStratum = st
}

func (s *Source) addSample(sm Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, sm)
	if len(s.samples) > maxSamples {
		s.samples = s.samples[len(s.samples)-maxSamples:]
	}
	s.accepted++
	s.lastReject = ""
	s.lastPollErr = ""
}

func (s *Source) setReject(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejected++
	s.lastReject = reason
}

func (s *Source) setPollErr(err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastPollErr = err
}

func (s *Source) lastAcceptedTx() NTPTime {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRespTx
}

func (s *Source) setLastAcceptedTx(tx NTPTime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRespTx = tx
}

// Candidate returns the freshest valid sample: among non-expired samples
// pick the minimum delay; ties go to the most recent.
func (s *Source) Candidate(maxAge time.Duration, now time.Time) (Sample, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var best Sample
	found := false
	for _, sm := range s.samples {
		if now.Sub(sm.RecvMono) > maxAge {
			continue // expired by monotonic clock
		}
		if !found || sm.Delay < best.Delay ||
			(sm.Delay == best.Delay && sm.RecvMono.After(best.RecvMono)) {
			best = sm
			found = true
		}
	}
	return best, found
}

// snapshot returns a copy of current samples for the HTTP API.
func (s *Source) snapshot() []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Sample, len(s.samples))
	copy(out, s.samples)
	return out
}
