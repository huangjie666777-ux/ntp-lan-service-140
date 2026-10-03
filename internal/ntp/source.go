package ntp

import (
	"sync"
	"time"
)

const maxSamples = 8

// Sample is one validated four-timestamp measurement.
type Sample struct {
	Offset    time.Duration
	Delay     time.Duration
	Stratum   uint8
	ReceiveAt time.Time // local monotonic receive time (t4)
}

// Rejection records why a packet or sample was discarded.
type Rejection struct {
	At     time.Time
	Reason string
}

const maxRejections = 8

// Source tracks samples and rejections for one upstream.
type Source struct {
	Addr string

	mu         sync.Mutex
	samples    []Sample // newest last, max 8
	rejections []Rejection
}

func NewSource(addr string) *Source { return &Source{Addr: addr} }

// addSample keeps the most recent maxSamples valid samples.
func (s *Source) addSample(sm Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, sm)
	if len(s.samples) > maxSamples {
		s.samples = s.samples[len(s.samples)-maxSamples:]
	}
}

func (s *Source) addRejection(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rejections = append(s.rejections, Rejection{At: time.Now(), Reason: reason})
	if len(s.rejections) > maxRejections {
		s.rejections = s.rejections[len(s.rejections)-maxRejections:]
	}
}

// fresh returns samples younger than validity, judged by the monotonic clock.
func (s *Source) fresh(validity time.Duration) []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Sample
	for _, sm := range s.samples {
		if time.Since(sm.ReceiveAt) <= validity {
			out = append(out, sm)
		}
	}
	return out
}

// Candidate returns the fresh sample with the smallest delay;
// ties go to the most recent sample. ok is false when none are fresh.
func (s *Source) Candidate(validity time.Duration) (Sample, bool) {
	fs := s.fresh(validity)
	if len(fs) == 0 {
		return Sample{}, false
	}
	best := fs[0]
	for _, sm := range fs[1:] {
		if sm.Delay < best.Delay || (sm.Delay == best.Delay && sm.ReceiveAt.After(best.ReceiveAt)) {
			best = sm
		}
	}
	return best, true
}

// Snapshot exposes state for the HTTP API.
type SourceSnapshot struct {
	Addr       string       `json:"addr"`
	Samples    []SampleView `json:"samples"`
	Rejections []Rejection  `json:"rejections"`
	Candidate  *SampleView  `json:"candidate,omitempty"`
}

type SampleView struct {
	Offset    time.Duration `json:"offsetNs"`
	Delay     time.Duration `json:"delayNs"`
	Stratum   uint8         `json:"stratum"`
	ReceiveAt time.Time     `json:"receiveAt"`
	Fresh     bool          `json:"fresh"`
}

func (s *Source) Snapshot(validity time.Duration) SourceSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := SourceSnapshot{Addr: s.Addr}
	for _, sm := range s.samples {
		snap.Samples = append(snap.Samples, SampleView{
			Offset: sm.Offset, Delay: sm.Delay, Stratum: sm.Stratum,
			ReceiveAt: sm.ReceiveAt, Fresh: time.Since(sm.ReceiveAt) <= validity,
		})
	}
	snap.Rejections = append(snap.Rejections, s.rejections...)
	return snap
}
