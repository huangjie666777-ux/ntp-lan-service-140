package main

import (
	"log"
	"sort"
	"time"
)

// Selection describes the outcome of one source-selection pass.
type Selection struct {
	Synced     bool
	SourceName string
	Offset     time.Duration
	Delay      time.Duration
	Stratum    uint8
	Agreeing   []string
}

// SelectSource applies median filtering across the three sources and
// updates the virtual clock. When conditions fail the clock becomes
// unsynchronized; old offsets are never reused.
func SelectSource(sources []*Source, cfg *Config, clock *VirtualClock) Selection {
	now := time.Now()
	type cand struct {
		src *Source
		sm  Sample
	}
	var cands []cand
	for _, s := range sources {
		sm, ok := s.Candidate(cfg.SampleMaxAge.Duration, now)
		if !ok {
			clock.Unset()
			log.Printf("select: source %s has no fresh candidate; unsynchronized", s.Name)
			return Selection{}
		}
		cands = append(cands, cand{s, sm})
	}

	// Median of the three offsets.
	offsets := []time.Duration{cands[0].sm.Offset, cands[1].sm.Offset, cands[2].sm.Offset}
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	median := offsets[1]

	// Keep sources within the agreement threshold of the median.
	var agreeing []cand
	for _, c := range cands {
		d := c.sm.Offset - median
		if d < 0 {
			d = -d
		}
		if d <= cfg.AgreeThreshold.Duration {
			agreeing = append(agreeing, c)
		}
	}
	if len(agreeing) < 2 {
		clock.Unset()
		log.Printf("select: only %d source(s) agree within %v; unsynchronized", len(agreeing), cfg.AgreeThreshold)
		return Selection{}
	}

	// Among agreeing sources: lowest delay wins; ties by config order.
	sort.SliceStable(agreeing, func(i, j int) bool {
		if agreeing[i].sm.Delay != agreeing[j].sm.Delay {
			return agreeing[i].sm.Delay < agreeing[j].sm.Delay
		}
		return agreeing[i].src.Order < agreeing[j].src.Order
	})
	best := agreeing[0]
	clock.Set(best.sm.Offset, best.src.Name, best.src.StratumHint())
	names := make([]string, 0, len(agreeing))
	for _, c := range agreeing {
		names = append(names, c.src.Name)
	}
	return Selection{
		Synced:     true,
		SourceName: best.src.Name,
		Offset:     best.sm.Offset,
		Delay:      best.sm.Delay,
		Stratum:    best.src.StratumHint(),
		Agreeing:   names,
	}
}
