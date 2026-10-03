package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// NTPv4 packet, 48 bytes, RFC 5905.
const (
	PacketLen      = 48
	ntpEpochOffset = 2208988800 // seconds between 1900 and 1970
	ModeClient     = 3
	ModeServer     = 4
	Version        = 4
	LIUnknown      = 3
	StratumInvalid = 16
)

type Packet struct {
	LI             uint8
	Version        uint8
	Mode           uint8
	Stratum        uint8
	Poll           int8
	Precision      int8
	RootDelay      uint32
	RootDispersion uint32
	ReferenceID    uint32
	ReferenceTime  NTPTime
	OriginateTime  NTPTime
	ReceiveTime    NTPTime
	TransmitTime   NTPTime
}

// NTPTime is a 64-bit NTP timestamp (32.32 fixed point, seconds since 1900).
type NTPTime struct {
	Seconds uint32
	Frac    uint32
}

func NTPTimeFromTime(t time.Time) NTPTime {
	sec := uint64(t.Unix()) + ntpEpochOffset
	frac := uint64(t.Nanosecond()) * (1 << 32) / 1e9
	return NTPTime{Seconds: uint32(sec), Frac: uint32(frac)}
}

// ToTime converts to wall-clock time; valid reports whether the
// timestamp is non-zero (zero is never a valid NTP timestamp here).
func (nt NTPTime) ToTime() (time.Time, bool) {
	if nt.Seconds == 0 && nt.Frac == 0 {
		return time.Time{}, false
	}
	sec := int64(nt.Seconds) - ntpEpochOffset
	nsec := int64(nt.Frac) * 1e9 / (1 << 32)
	return time.Unix(sec, nsec).UTC(), true
}

func (p *Packet) Marshal() []byte {
	b := make([]byte, PacketLen)
	b[0] = p.LI<<6 | p.Version<<3 | p.Mode
	b[1] = p.Stratum
	b[2] = uint8(p.Poll)
	b[3] = uint8(p.Precision)
	binary.BigEndian.PutUint32(b[4:], p.RootDelay)
	binary.BigEndian.PutUint32(b[8:], p.RootDispersion)
	binary.BigEndian.PutUint32(b[12:], p.ReferenceID)
	binary.BigEndian.PutUint32(b[16:], p.ReferenceTime.Seconds)
	binary.BigEndian.PutUint32(b[20:], p.ReferenceTime.Frac)
	binary.BigEndian.PutUint32(b[24:], p.OriginateTime.Seconds)
	binary.BigEndian.PutUint32(b[28:], p.OriginateTime.Frac)
	binary.BigEndian.PutUint32(b[32:], p.ReceiveTime.Seconds)
	binary.BigEndian.PutUint32(b[36:], p.ReceiveTime.Frac)
	binary.BigEndian.PutUint32(b[40:], p.TransmitTime.Seconds)
	binary.BigEndian.PutUint32(b[44:], p.TransmitTime.Frac)
	return b
}

var ErrShortPacket = errors.New("packet shorter than 48 bytes")

func ParsePacket(b []byte) (*Packet, error) {
	if len(b) < PacketLen {
		return nil, ErrShortPacket
	}
	p := &Packet{
		LI:        b[0] >> 6,
		Version:   (b[0] >> 3) & 0x7,
		Mode:      b[0] & 0x7,
		Stratum:   b[1],
		Poll:      int8(b[2]),
		Precision: int8(b[3]),
	}
	p.RootDelay = binary.BigEndian.Uint32(b[4:])
	p.RootDispersion = binary.BigEndian.Uint32(b[8:])
	p.ReferenceID = binary.BigEndian.Uint32(b[12:])
	p.ReferenceTime = NTPTime{binary.BigEndian.Uint32(b[16:]), binary.BigEndian.Uint32(b[20:])}
	p.OriginateTime = NTPTime{binary.BigEndian.Uint32(b[24:]), binary.BigEndian.Uint32(b[28:])}
	p.ReceiveTime = NTPTime{binary.BigEndian.Uint32(b[32:]), binary.BigEndian.Uint32(b[36:])}
	p.TransmitTime = NTPTime{binary.BigEndian.Uint32(b[40:]), binary.BigEndian.Uint32(b[44:])}
	return p, nil
}

// ValidateResponse checks a server response against the request we sent.
// Returns a human-readable rejection reason or "" if accepted.
func ValidateResponse(resp *Packet, reqTransmit NTPTime) string {
	if resp.Version != Version {
		return fmt.Sprintf("bad version %d (want 4)", resp.Version)
	}
	if resp.Mode != ModeServer {
		return fmt.Sprintf("bad mode %d (want 4/server)", resp.Mode)
	}
	if resp.Stratum < 1 || resp.Stratum > 14 {
		return fmt.Sprintf("bad stratum %d (want 1..14)", resp.Stratum)
	}
	if resp.LI == LIUnknown {
		return "leap indicator 3 (unsynchronized)"
	}
	if resp.OriginateTime != reqTransmit {
		return "originate timestamp does not match request transmit"
	}
	return ""
}

// ComputeOffsetDelay derives offset and delay from the four timestamps.
// t1: client send, t2: server receive, t3: server transmit, t4: client receive.
func ComputeOffsetDelay(t1, t2, t3, t4 time.Time) (offset, delay time.Duration, err error) {
	delay = t4.Sub(t1) - t3.Sub(t2)
	if delay < 0 {
		return 0, 0, fmt.Errorf("negative delay %v", delay)
	}
	offset = (t2.Sub(t1) + t3.Sub(t4)) / 2
	return offset, delay, nil
}
