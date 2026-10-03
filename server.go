package main

import (
	"context"
	"log"
	"net"
	"time"
)

// UDPServer answers NTP client (mode 3) requests.
type UDPServer struct {
	addr  string
	clock *VirtualClock
	conn  *net.UDPConn
}

func NewUDPServer(addr string, clock *VirtualClock) *UDPServer {
	return &UDPServer{addr: addr, clock: clock}
}

func (s *UDPServer) Run(ctx context.Context) error {
	laddr, err := net.ResolveUDPAddr("udp4", s.addr)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp4", laddr)
	if err != nil {
		return err
	}
	s.conn = conn
	log.Printf("udp: serving NTP on %s", conn.LocalAddr())

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	buf := make([]byte, 512)
	for {
		n, raddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil // shutting down
			}
			continue
		}
		recvAt := time.Now()
		go s.handle(buf[:n], raddr, recvAt)
	}
}

func (s *UDPServer) handle(raw []byte, raddr *net.UDPAddr, recvAt time.Time) {
	req, err := ParsePacket(raw)
	if err != nil || req.Mode != ModeClient {
		return // only mode 3 client requests are answered
	}
	synced, offset, _, stratum := s.clock.State()
	resp := &Packet{
		Version:       Version,
		Mode:          ModeServer,
		OriginateTime: req.TransmitTime, // echoed verbatim
		Poll:          req.Poll,
		Precision:     -20,
	}
	if synced {
		resp.LI = 0
		resp.Stratum = stratum + 1
		// Receive/transmit timestamps use local time + selected offset.
		resp.ReceiveTime = NTPTimeFromTime(recvAt.Add(offset))
		resp.TransmitTime = NTPTimeFromTime(time.Now().Add(offset))
		resp.ReferenceTime = resp.ReceiveTime
		resp.ReferenceID = 0x4c4f434c // "LOCL"
	} else {
		// Never impersonate reliable time.
		resp.LI = LIUnknown
		resp.Stratum = StratumInvalid
	}
	if _, err := s.conn.WriteToUDP(resp.Marshal(), raddr); err != nil {
		log.Printf("udp: write to %s: %v", raddr, err)
	}
}
