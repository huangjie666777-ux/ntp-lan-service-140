package ntp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Poller periodically queries all upstreams concurrently over real UDP.
type Poller struct {
	sources   []*Source
	interval  time.Duration
	timeout   time.Duration
	validity  time.Duration
	threshold time.Duration
	clock     *Clock

	mu  sync.Mutex
	sel Selection
}

func NewPoller(sources []*Source, interval, timeout, validity, threshold time.Duration, clock *Clock) *Poller {
	return &Poller{sources: sources, interval: interval, timeout: timeout, validity: validity, threshold: threshold, clock: clock}
}

func (p *Poller) Selection() Selection {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sel
}

// Run polls until ctx is cancelled; the first round starts immediately.
func (p *Poller) Run(ctx context.Context) {
	p.round()
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.round()
		}
	}
}

func (p *Poller) round() {
	var wg sync.WaitGroup
	for _, s := range p.sources {
		wg.Add(1)
		go func(s *Source) {
			defer wg.Done()
			p.pollOne(s)
		}(s)
	}
	wg.Wait()
	sel := Select(p.sources, p.validity, p.threshold)
	p.mu.Lock()
	p.sel = sel
	p.mu.Unlock()
	var stratum uint8
	if sel.Synchronized {
		if c, ok := p.sources[sel.Chosen].Candidate(p.validity); ok {
			stratum = c.Stratum
		}
	}
	p.clock.Update(sel, stratum)
}

// pollOne sends one 48-byte NTPv4 client request and validates the reply.
func (p *Poller) pollOne(s *Source) {
	raddr, err := net.ResolveUDPAddr("udp4", s.Addr)
	if err != nil {
		s.addRejection("resolve: " + err.Error())
		return
	}
	conn, err := net.DialUDP("udp4", nil, raddr)
	if err != nil {
		s.addRejection("dial: " + err.Error())
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(p.timeout))

	req := &Packet{
		Version:      Version,
		Mode:         ModeClient,
		TransmitTime: TimeToTimestamp(time.Now()),
	}
	t1 := time.Now()
	if _, err := conn.Write(req.Encode()); err != nil {
		s.addRejection("write: " + err.Error())
		return
	}

	buf := make([]byte, 512)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				s.addRejection("timeout waiting for reply")
			} else {
				s.addRejection("read: " + err.Error())
			}
			return
		}
		t4 := time.Now()
		resp, err := DecodePacket(buf[:n])
		if err != nil {
			s.addRejection("malformed packet: " + err.Error())
			continue
		}
		if reason := validateResponse(req, resp); reason != "" {
			s.addRejection(reason)
			continue
		}
		sm, reason := computeSample(req.TransmitTime, resp, t1, t4)
		if reason != "" {
			s.addRejection(reason)
			continue
		}
		s.addSample(sm)
		return
	}
}

// validateResponse enforces: version 4, mode 4 (server), stratum 1-14,
// LI != 3, and Originate matching the request Transmit (drops late and
// duplicate packets from earlier exchanges).
func validateResponse(req, resp *Packet) string {
	if resp.Version != Version {
		return fmt.Sprintf("bad version %d", resp.Version)
	}
	if resp.Mode != ModeServer {
		return fmt.Sprintf("bad mode %d", resp.Mode)
	}
	if resp.Stratum < 1 || resp.Stratum > 14 {
		return fmt.Sprintf("bad stratum %d", resp.Stratum)
	}
	if resp.LI == 3 {
		return "leap indicator 3 (unsynchronized)"
	}
	if resp.OriginateTime != req.TransmitTime {
		return "originate mismatch (late/duplicate/foreign packet)"
	}
	return ""
}

// computeSample applies the four-timestamp offset/delay formulas and
// rejects negative delay and invalid (zero or non-monotonic) timestamps.
func computeSample(t1ts Timestamp, resp *Packet, t1, t4 time.Time) (Sample, string) {
	t2ts, t3ts := resp.ReceiveTime, resp.TransmitTime
	if t1ts.IsZero() || t2ts.IsZero() || t3ts.IsZero() {
		return Sample{}, "invalid zero timestamp in response"
	}
	t2, t3 := t2ts.ToTime(), t3ts.ToTime()
	if t3.Before(t2) {
		return Sample{}, "invalid timestamps: transmit before receive"
	}
	offset := (t2.Sub(t1) + t3.Sub(t4)) / 2
	delay := t4.Sub(t1) - t3.Sub(t2)
	if delay < 0 {
		return Sample{}, "negative delay rejected"
	}
	return Sample{Offset: offset, Delay: delay, Stratum: resp.Stratum, ReceiveAt: t4}, ""
}
