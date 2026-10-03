package ntp

import (
	"testing"
	"time"
)

func TestPacketRoundTrip(t *testing.T) {
	p := &Packet{LI: 1, Version: 4, Mode: 4, Stratum: 2, Poll: 6, Precision: -20,
		OriginateTime: TimeToTimestamp(time.Now()), TransmitTime: TimeToTimestamp(time.Now())}
	q, err := DecodePacket(p.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if q.LI != 1 || q.Version != 4 || q.Mode != 4 || q.Stratum != 2 {
		t.Fatalf("header mismatch: %+v", q)
	}
	if q.OriginateTime != p.OriginateTime {
		t.Fatal("originate not preserved")
	}
	if _, err := DecodePacket(make([]byte, 10)); err != ErrShortPacket {
		t.Fatal("expected ErrShortPacket")
	}
}

func TestValidateResponse(t *testing.T) {
	req := &Packet{TransmitTime: 12345}
	base := func() *Packet {
		return &Packet{Version: 4, Mode: 4, Stratum: 2, OriginateTime: 12345}
	}
	if r := validateResponse(req, base()); r != "" {
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
		{func(p *Packet) { p.OriginateTime = 999 }}, // late/duplicate
	}
	for i, c := range cases {
		p := base()
		c.mutate(p)
		if r := validateResponse(req, p); r == "" {
			t.Fatalf("case %d: invalid response accepted", i)
		}
	}
}

func TestComputeSample(t *testing.T) {
	t1 := time.Now()
	t2 := t1.Add(100 * time.Millisecond)
	t3 := t2.Add(10 * time.Millisecond)
	t4 := t1.Add(120 * time.Millisecond)
	resp := &Packet{ReceiveTime: TimeToTimestamp(t2), TransmitTime: TimeToTimestamp(t3), Stratum: 3}
	sm, reason := computeSample(TimeToTimestamp(t1), resp, t1, t4)
	if reason != "" {
		t.Fatal(reason)
	}
	if sm.Delay < 0 {
		t.Fatal("negative delay")
	}
	// zero receive timestamp must be rejected
	resp.ReceiveTime = 0
	if _, reason := computeSample(TimeToTimestamp(t1), resp, t1, t4); reason == "" {
		t.Fatal("zero timestamp accepted")
	}
}

func TestSourceCandidateAndExpiry(t *testing.T) {
	s := NewSource("127.0.0.1:1")
	old := Sample{Offset: time.Millisecond, Delay: time.Millisecond, ReceiveAt: time.Now().Add(-time.Hour)}
	s.addSample(old)
	if _, ok := s.Candidate(time.Minute); ok {
		t.Fatal("stale sample treated as fresh")
	}
	for i := 0; i < 10; i++ {
		s.addSample(Sample{Offset: time.Duration(i) * time.Millisecond, Delay: time.Duration(10-i) * time.Millisecond, ReceiveAt: time.Now()})
	}
	c, ok := s.Candidate(time.Minute)
	if !ok {
		t.Fatal("no candidate")
	}
	if c.Delay != time.Millisecond {
		t.Fatalf("expected min delay 1ms, got %s", c.Delay)
	}
	if len(s.fresh(time.Minute)) != maxSamples {
		t.Fatal("ring did not cap at 8 samples")
	}
}

func TestSelectConsensus(t *testing.T) {
	mk := func(offset, delay time.Duration) *Source {
		s := NewSource("x")
		s.addSample(Sample{Offset: offset, Delay: delay, ReceiveAt: time.Now()})
		return s
	}
	// two agree, one outlier -> synchronized, outlier excluded
	srcs := []*Source{mk(100*time.Millisecond, 5*time.Millisecond), mk(120*time.Millisecond, 2*time.Millisecond), mk(5*time.Second, time.Millisecond)}
	sel := Select(srcs, time.Minute, 500*time.Millisecond)
	if !sel.Synchronized {
		t.Fatal("expected synchronization")
	}
	if sel.Chosen != 1 {
		t.Fatalf("expected source 1 (min delay among agreeing), got %d", sel.Chosen)
	}
	// all three disagree -> not synchronized
	srcs = []*Source{mk(0, time.Millisecond), mk(2*time.Second, time.Millisecond), mk(9*time.Second, time.Millisecond)}
	if sel := Select(srcs, time.Minute, 500*time.Millisecond); sel.Synchronized {
		t.Fatal("should not synchronize without 2 agreeing sources")
	}
	// missing fresh candidate -> not synchronized
	stale := NewSource("y")
	stale.addSample(Sample{Offset: 0, Delay: time.Millisecond, ReceiveAt: time.Now().Add(-time.Hour)})
	srcs = []*Source{mk(0, time.Millisecond), mk(time.Millisecond, time.Millisecond), stale}
	if sel := Select(srcs, time.Minute, 500*time.Millisecond); sel.Synchronized {
		t.Fatal("should not synchronize with a stale source")
	}
}

func TestClockDropsOffsetWhenUnsynchronized(t *testing.T) {
	c := &Clock{}
	c.Update(Selection{Synchronized: true, Offset: time.Second}, 2)
	if !c.Synchronized() || c.Offset() != time.Second {
		t.Fatal("expected synced offset")
	}
	c.Update(Selection{Synchronized: false, Chosen: -1}, 0)
	if c.Synchronized() || c.Offset() != 0 {
		t.Fatal("stale offset must not be reused")
	}
}
