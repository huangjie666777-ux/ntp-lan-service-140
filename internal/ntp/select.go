package ntp

import (
	"sort"
	"time"
)

// Selection is the outcome of one consensus round.
type Selection struct {
	Synchronized bool
	Offset       time.Duration // virtual offset of the chosen source
	Chosen       int           // index into sources, -1 when unsynchronized
	Agreeing     []int         // sources within threshold of the median
}

// Select implements the consensus rule:
//   - every source must offer a fresh candidate (min delay, latest on tie);
//   - center on the median offset, keep sources within threshold;
//   - require at least two agreeing sources, else unsynchronized;
//   - among the agreeing sources pick the smallest delay, ties by config order.
func Select(sources []*Source, validity, threshold time.Duration) Selection {
	cands := make([]Sample, len(sources))
	for i, s := range sources {
		c, ok := s.Candidate(validity)
		if !ok {
			return Selection{Synchronized: false, Chosen: -1}
		}
		cands[i] = c
	}

	offsets := make([]time.Duration, len(cands))
	for i, c := range cands {
		offsets[i] = c.Offset
	}
	sorted := append([]time.Duration(nil), offsets...)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a] < sorted[b] })
	median := sorted[len(sorted)/2]

	var agreeing []int
	for i, off := range offsets {
		d := off - median
		if d < 0 {
			d = -d
		}
		if d <= threshold {
			agreeing = append(agreeing, i)
		}
	}
	if len(agreeing) < 2 {
		return Selection{Synchronized: false, Chosen: -1, Agreeing: agreeing}
	}

	chosen := agreeing[0]
	for _, i := range agreeing[1:] {
		if cands[i].Delay < cands[chosen].Delay {
			chosen = i
		}
	}
	return Selection{
		Synchronized: true,
		Offset:       cands[chosen].Offset,
		Chosen:       chosen,
		Agreeing:     agreeing,
	}
}
