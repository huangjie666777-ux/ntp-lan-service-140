package ntp

import (
	"encoding/binary"
	"errors"
	"time"
)

const (
	PacketSize     = 48
	Version        = 4
	ModeClient     = 3
	ModeServer     = 4
	ntpEpochOffset = 2208988800 // seconds between 1900 and 1970
	fracPerSecond  = 1 << 32
)

// Timestamp is an NTP 64-bit fixed-point timestamp (seconds.fraction since 1900).
type Timestamp uint64

func TimeToTimestamp(t time.Time) Timestamp {
	sec := uint64(t.Unix() + ntpEpochOffset)
	frac := uint64(t.Nanosecond()) * fracPerSecond / 1e9
	return Timestamp(sec<<32 | frac)
}

func (ts Timestamp) ToTime() time.Time {
	sec := int64(uint64(ts) >> 32)
	frac := uint64(ts) & 0xffffffff
	return time.Unix(sec-ntpEpochOffset, int64(frac*1e9/fracPerSecond)).UTC()
}

func (ts Timestamp) IsZero() bool { return ts == 0 }

// Packet is a 48-byte NTPv4 packet.
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
	ReferenceTime  Timestamp
	OriginateTime  Timestamp
	ReceiveTime    Timestamp
	TransmitTime   Timestamp
}

var ErrShortPacket = errors.New("ntp: packet shorter than 48 bytes")

func DecodePacket(b []byte) (*Packet, error) {
	if len(b) < PacketSize {
		return nil, ErrShortPacket
	}
	p := &Packet{
		LI:             b[0] >> 6,
		Version:        (b[0] >> 3) & 0x7,
		Mode:           b[0] & 0x7,
		Stratum:        b[1],
		Poll:           int8(b[2]),
		Precision:      int8(b[3]),
		RootDelay:      binary.BigEndian.Uint32(b[4:8]),
		RootDispersion: binary.BigEndian.Uint32(b[8:12]),
		ReferenceID:    binary.BigEndian.Uint32(b[12:16]),
		ReferenceTime:  Timestamp(binary.BigEndian.Uint64(b[16:24])),
		OriginateTime:  Timestamp(binary.BigEndian.Uint64(b[24:32])),
		ReceiveTime:    Timestamp(binary.BigEndian.Uint64(b[32:40])),
		TransmitTime:   Timestamp(binary.BigEndian.Uint64(b[40:48])),
	}
	return p, nil
}

func (p *Packet) Encode() []byte {
	b := make([]byte, PacketSize)
	b[0] = p.LI<<6 | p.Version<<3 | p.Mode
	b[1] = p.Stratum
	b[2] = byte(p.Poll)
	b[3] = byte(p.Precision)
	binary.BigEndian.PutUint32(b[4:8], p.RootDelay)
	binary.BigEndian.PutUint32(b[8:12], p.RootDispersion)
	binary.BigEndian.PutUint32(b[12:16], p.ReferenceID)
	binary.BigEndian.PutUint64(b[16:24], uint64(p.ReferenceTime))
	binary.BigEndian.PutUint64(b[24:32], uint64(p.OriginateTime))
	binary.BigEndian.PutUint64(b[32:40], uint64(p.ReceiveTime))
	binary.BigEndian.PutUint64(b[40:48], uint64(p.TransmitTime))
	return b
}
