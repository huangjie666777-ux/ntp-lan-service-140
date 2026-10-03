package main

import (
	"testing"
	"time"
)

func TestPacketRoundTrip(t *testing.T) {
	p := &Packet{LI: 0, Version: 4, Mode: 4, Stratum: 2,
		TransmitTime: NTPTimeFromTime(time.Now())}
	q, err := ParsePacket(p.Marshal())
	if err != nil {
		t.Fatal(err)
	}
	if q.Version != 4 || q.Mode != 4 || q.Stratum != 2 {
		t.Fatalf("bad header: %+v", q)
	}
	if q.TransmitTime != p.TransmitTime {
		t.Fatal("transmit mismatch")
	}
}

func TestValidateResponse(t *testing.T) {
	tx := NTPTime{Seconds: 123, Frac: 456}
	base := &Packet{Version: 4, Mode: 4, Stratum: 1, OriginateTime: tx}
	if r := ValidateResponse(base, tx); r != "" {
		t.Fatalf("valid response rejected: %s", r)
	}
	cases := []struct {
		mutate func(*Packet)
	}{
		{func(p *Packet) { p.Version = 3 }},
		{func(p *Packet) { p.Mode = 3 }},
		{func(p *Packet) { p.Stratum = 0 }},
		{func(p *Packet) { p.Stratum = 15 }},
		{func(p *Packet) { p.LI = 3 }},
		{func(p *Packet) { p.OriginateTime = NTPTime{1, 1} }},
	}
	for i, c := range cases {
		p := *base
		c.mutate(&p)
		if r := ValidateResponse(&p, tx); r == "" {
			t.Fatalf("case %d: invalid response accepted", i)
		}
	}
}

func TestComputeOffsetDelay(t *testing.T) {
	t0 := time.Now()
	// Server clock +100ms, 20ms one-way delay each direction.
	off := 100 * time.Millisecond
	t1 := t0
	t2 := t1.Add(20*time.Millisecond + off)
	t3 := t2.Add(5 * time.Millisecond)
	t4 := t3.Add(20*time.Millisecond - off)
	offset, delay, err := ComputeOffsetDelay(t1, t2, t3, t4)
	if err != nil {
		t.Fatal(err)
	}
	if offset != off {
		t.Fatalf("offset=%v want %v", offset, off)
	}
	if delay != 40*time.Millisecond {
		t.Fatalf("delay=%v want 40ms", delay)
	}
	// Negative delay rejected.
	if _, _, err := ComputeOffsetDelay(t1, t2, t2, t1.Add(-time.Second)); err == nil {
		t.Fatal("negative delay accepted")
	}
}

func TestSourceCandidate(t *testing.T) {
	s := &Source{}
	now := time.Now()
	s.addSample(Sample{Offset: 1, Delay: 50 * time.Millisecond, RecvMono: now.Add(-time.Minute)})
	s.addSample(Sample{Offset: 2, Delay: 30 * time.Millisecond, RecvMono: now.Add(-time.Second)})
	s.addSample(Sample{Offset: 3, Delay: 30 * time.Millisecond, RecvMono: now})
	c, ok := s.Candidate(10*time.Second, now)
	if !ok {
		t.Fatal("no candidate")
	}
	// Expired 50ms sample ignored; tie on 30ms delay -> latest wins.
	if c.Offset != 3 {
		t.Fatalf("candidate offset=%v want 3", c.Offset)
	}
	if _, ok := s.Candidate(time.Millisecond, now.Add(time.Hour)); ok {
		t.Fatal("expired candidate returned")
	}
}

func TestSelectSource(t *testing.T) {
	cfg := &Config{SampleMaxAge: Duration{time.Minute}, AgreeThreshold: Duration{50 * time.Millisecond}}
	clock := &VirtualClock{}
	mk := func(order int, off, dly time.Duration) *Source {
		s := &Source{Name: string(rune('a' + order)), Order: order}
		s.addSample(Sample{Offset: off, Delay: dly, RecvMono: time.Now()})
		return s
	}
	// Two agree (100ms, 110ms), one skewed (+10s) -> excluded; lowest delay wins.
	srcs := []*Source{
		mk(0, 100*time.Millisecond, 20*time.Millisecond),
		mk(1, 110*time.Millisecond, 10*time.Millisecond),
		mk(2, 10*time.Second, 5*time.Millisecond),
	}
	sel := SelectSource(srcs, cfg, clock)
	if !sel.Synced || sel.SourceName != "b" {
		t.Fatalf("sel=%+v", sel)
	}
	if len(sel.Agreeing) != 2 {
		t.Fatalf("agreeing=%v", sel.Agreeing)
	}
	// All three disagree -> unsynchronized, no offset reuse.
	srcs[1] = mk(1, 5*time.Second, 10*time.Millisecond)
	if sel := SelectSource(srcs, cfg, clock); sel.Synced {
		t.Fatal("should be unsynchronized")
	}
	if synced, off, _, _ := clock.State(); synced || off != 0 {
		t.Fatal("old offset reused")
	}
	// Stale sample -> unsynchronized.
	stale := mk(0, 100*time.Millisecond, 20*time.Millisecond)
	stale.samples[0].RecvMono = time.Now().Add(-time.Hour)
	srcs[1] = stale
	if sel := SelectSource(srcs, cfg, clock); sel.Synced {
		t.Fatal("stale sample should unsync")
	}
}

func TestSampleRingLimit(t *testing.T) {
	s := &Source{}
	for i := 0; i < 20; i++ {
		s.addSample(Sample{Offset: time.Duration(i), Delay: time.Millisecond, RecvMono: time.Now()})
	}
	if got := len(s.snapshot()); got != 8 {
		t.Fatalf("samples=%d want 8", got)
	}
}
