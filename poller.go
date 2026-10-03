package main

import (
	"context"
	"net"
	"sync"
	"time"
)

// Poller concurrently queries all upstreams with real UDP NTPv4 requests.
type Poller struct {
	cfg     *Config
	sources []*Source
	clock   *VirtualClock
}

func NewPoller(cfg *Config, clock *VirtualClock) *Poller {
	p := &Poller{cfg: cfg, clock: clock}
	for i, u := range cfg.Upstreams {
		p.sources = append(p.sources, &Source{Name: u.Name, Addr: u.Address, Order: i})
	}
	return p
}

// Run polls until ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.PollInterval.Duration)
	defer ticker.Stop()
	p.pollAll()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollAll()
		}
	}
}

func (p *Poller) pollAll() {
	var wg sync.WaitGroup
	for _, src := range p.sources {
		wg.Add(1)
		go func(s *Source) {
			defer wg.Done()
			p.pollOne(s)
		}(src)
	}
	wg.Wait()
	SelectSource(p.sources, p.cfg, p.clock)
}

// pollOne sends one NTPv4 client request over UDP and validates the reply.
func (p *Poller) pollOne(s *Source) {
	raddr, err := net.ResolveUDPAddr("udp4", s.Addr)
	if err != nil {
		s.setPollErr("resolve: " + err.Error())
		return
	}
	conn, err := net.DialUDP("udp4", nil, raddr)
	if err != nil {
		s.setPollErr("dial: " + err.Error())
		return
	}
	defer conn.Close()
	// Late packets (arriving after the timeout) never become samples:
	// the socket is closed when the deadline passes.
	_ = conn.SetReadDeadline(time.Now().Add(p.cfg.PollTimeout.Duration))

	req := &Packet{Version: Version, Mode: ModeClient, Stratum: StratumInvalid}
	t1 := time.Now()
	req.TransmitTime = NTPTimeFromTime(t1)
	if _, err := conn.Write(req.Marshal()); err != nil {
		s.setPollErr("write: " + err.Error())
		return
	}
	s.mu.Lock()
	s.polls++
	s.mu.Unlock()

	buf := make([]byte, 512)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			s.setPollErr("read: " + err.Error())
			return
		}
		t4 := time.Now()
		resp, err := ParsePacket(buf[:n])
		if err != nil {
			s.setReject(err.Error())
			continue
		}
		// Duplicate response to an already-accepted exchange: ignore.
		if resp.TransmitTime == s.lastAcceptedTx() && resp.TransmitTime != (NTPTime{}) {
			continue
		}
		if reason := ValidateResponse(resp, req.TransmitTime); reason != "" {
			s.setReject(reason)
			continue
		}
		t2, ok1 := resp.ReceiveTime.ToTime()
		t3, ok2 := resp.TransmitTime.ToTime()
		if !ok1 || !ok2 {
			s.setReject("invalid (zero) receive/transmit timestamp")
			continue
		}
		offset, delay, err := ComputeOffsetDelay(t1, t2, t3, t4)
		if err != nil {
			s.setReject(err.Error())
			continue
		}
		s.setLastAcceptedTx(resp.TransmitTime)
		s.setStratum(resp.Stratum)
		s.addSample(Sample{Offset: offset, Delay: delay, RecvMono: t4, RecvWall: t4})
		return
	}
}
