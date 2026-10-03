package ntp

import (
	"context"
	"net"
	"time"
)

// Server answers mode-3 client requests with mode-4 responses.
type Server struct {
	clock *Clock
	conn  *net.UDPConn
}

func NewServer(clock *Clock) *Server { return &Server{clock: clock} }

// Listen binds the UDP socket; the socket is released on Close.
func (s *Server) Listen(addr string) error {
	a, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return err
	}
	c, err := net.ListenUDP("udp4", a)
	if err != nil {
		return err
	}
	s.conn = c
	return nil
}

func (s *Server) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// Serve handles requests until ctx is cancelled or Close is called.
func (s *Server) Serve(ctx context.Context) {
	go func() { <-ctx.Done(); s.Close() }()
	buf := make([]byte, 512)
	for {
		n, raddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return // socket closed
		}
		req, err := DecodePacket(buf[:n])
		if err != nil {
			continue
		}
		if req.Mode != ModeClient {
			continue
		}
		s.conn.WriteToUDP(s.reply(req).Encode(), raddr)
	}
}

// reply builds a mode-4 response, copying Originate verbatim. When
// synchronized, receive/transmit use virtual time and stratum is the
// selected source's stratum + 1. Otherwise LI=3 and stratum=16 so we
// never pose as a reliable time source.
func (s *Server) reply(req *Packet) *Packet {
	resp := &Packet{
		Version:       Version,
		Mode:          ModeServer,
		Poll:          req.Poll,
		Precision:     -20,
		OriginateTime: req.TransmitTime, // copied verbatim
	}
	if s.clock.Synchronized() {
		now := s.clock.Now()
		resp.LI = 0
		resp.Stratum = s.clock.Stratum + 1
		resp.ReceiveTime = TimeToTimestamp(now)
		resp.TransmitTime = TimeToTimestamp(now)
		resp.ReferenceTime = TimeToTimestamp(now)
		resp.ReferenceID = 0x4c4f434c // "LOCL"
	} else {
		now := time.Now()
		resp.LI = 3
		resp.Stratum = 16
		resp.ReceiveTime = TimeToTimestamp(now)
		resp.TransmitTime = TimeToTimestamp(now)
	}
	return resp
}
