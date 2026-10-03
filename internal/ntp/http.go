package ntp

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// StatusHandler exposes poller/clock state identical to UDP behavior.
type StatusHandler struct {
	sources  []*Source
	poller   *Poller
	clock    *Clock
	validity time.Duration
}

func NewStatusHandler(sources []*Source, poller *Poller, clock *Clock, validity time.Duration) *StatusHandler {
	return &StatusHandler{sources: sources, poller: poller, clock: clock, validity: validity}
}

func (h *StatusHandler) Router() http.Handler {
	r := chi.NewRouter()
	r.Get("/status", h.status)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return r
}

type statusResponse struct {
	Synchronized bool             `json:"synchronized"`
	OffsetNs     time.Duration    `json:"offsetNs"`
	Chosen       int              `json:"chosen"`
	Agreeing     []int            `json:"agreeing"`
	Stratum      uint8            `json:"stratum"`
	VirtualNow   time.Time        `json:"virtualNow"`
	Sources      []SourceSnapshot `json:"sources"`
}

func (h *StatusHandler) status(w http.ResponseWriter, _ *http.Request) {
	sel := h.poller.Selection()
	resp := statusResponse{
		Synchronized: h.clock.Synchronized(),
		OffsetNs:     h.clock.Offset(),
		Chosen:       sel.Chosen,
		Agreeing:     sel.Agreeing,
		VirtualNow:   h.clock.Now(),
	}
	if resp.Synchronized {
		resp.Stratum = h.clock.Stratum + 1
	}
	for _, s := range h.sources {
		snap := s.Snapshot(h.validity)
		if c, ok := s.Candidate(h.validity); ok {
			snap.Candidate = &SampleView{Offset: c.Offset, Delay: c.Delay, Stratum: c.Stratum, ReceiveAt: c.ReceiveAt, Fresh: true}
		}
		resp.Sources = append(resp.Sources, snap)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
