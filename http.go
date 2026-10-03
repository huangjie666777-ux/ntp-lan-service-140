package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

type sampleJSON struct {
	Offset   string `json:"offset"`
	Delay    string `json:"delay"`
	Received string `json:"received"`
	Fresh    bool   `json:"fresh"`
}

type sourceJSON struct {
	Name        string       `json:"name"`
	Address     string       `json:"address"`
	Stratum     uint8        `json:"stratum"`
	Polls       uint64       `json:"polls"`
	Accepted    uint64       `json:"accepted"`
	Rejected    uint64       `json:"rejected"`
	LastReject  string       `json:"last_reject_reason,omitempty"`
	LastPollErr string       `json:"last_poll_error,omitempty"`
	Candidate   *sampleJSON  `json:"candidate,omitempty"`
	Samples     []sampleJSON `json:"samples"`
}

type statusJSON struct {
	Synced    bool         `json:"synced"`
	Offset    string       `json:"offset"`
	Source    string       `json:"source,omitempty"`
	Stratum   uint8        `json:"stratum,omitempty"`
	ClockTime string       `json:"clock_time"`
	Sources   []sourceJSON `json:"sources"`
}

// NewHTTPHandler exposes poller/clock state consistent with UDP behavior.
func NewHTTPHandler(p *Poller, clock *VirtualClock, cfg *Config) http.Handler {
	r := chi.NewRouter()
	r.Get("/status", func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now()
		synced, offset, srcName, stratum := clock.State()
		st := statusJSON{Synced: synced, Offset: offset.String(), Source: srcName,
			Stratum: stratum, ClockTime: clock.Now().Format(time.RFC3339Nano)}
		for _, s := range p.sources {
			sj := sourceJSON{Name: s.Name, Address: s.Addr, Stratum: s.StratumHint()}
			s.mu.Lock()
			sj.Polls, sj.Accepted, sj.Rejected = s.polls, s.accepted, s.rejected
			sj.LastReject, sj.LastPollErr = s.lastReject, s.lastPollErr
			s.mu.Unlock()
			for _, sm := range s.snapshot() {
				sj.Samples = append(sj.Samples, sampleJSON{
					Offset:   sm.Offset.String(),
					Delay:    sm.Delay.String(),
					Received: sm.RecvWall.Format(time.RFC3339Nano),
					Fresh:    now.Sub(sm.RecvMono) <= cfg.SampleMaxAge.Duration,
				})
			}
			if sj.Samples == nil {
				sj.Samples = []sampleJSON{}
			}
			if c, ok := s.Candidate(cfg.SampleMaxAge.Duration, now); ok {
				sj.Candidate = &sampleJSON{Offset: c.Offset.String(), Delay: c.Delay.String(),
					Received: c.RecvWall.Format(time.RFC3339Nano), Fresh: true}
			}
			st.Sources = append(st.Sources, sj)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st)
	})
	return r
}
