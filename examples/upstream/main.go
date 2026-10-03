// Command upstream is a local test NTP server with a configurable clock
// offset and misbehavior modes, used to demonstrate filtering of bad sources.
package main

import (
	"flag"
	"log"
	"net"
	"time"

	"github.com/huangjie666777-ux/ntp-lan-service-140/internal/ntp"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:11230", "UDP listen address")
	offset := flag.Duration("offset", 0, "virtual clock offset to serve")
	stratum := flag.Uint("stratum", 1, "stratum to report")
	li := flag.Uint("li", 0, "leap indicator to report (3 = unsynchronized)")
	mode := flag.Uint("mode", 4, "mode to respond with")
	flag.Parse()

	a, err := net.ResolveUDPAddr("udp4", *addr)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.ListenUDP("udp4", a)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	log.Printf("upstream on %s offset=%s stratum=%d li=%d mode=%d", *addr, *offset, *stratum, *li, *mode)

	buf := make([]byte, 512)
	for {
		n, raddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		req, err := ntp.DecodePacket(buf[:n])
		if err != nil || req.Mode != ntp.ModeClient {
			continue
		}
		now := time.Now().Add(*offset)
		resp := &ntp.Packet{
			LI:            uint8(*li),
			Version:       ntp.Version,
			Mode:          uint8(*mode),
			Stratum:       uint8(*stratum),
			Poll:          req.Poll,
			Precision:     -20,
			OriginateTime: req.TransmitTime,
			ReceiveTime:   ntp.TimeToTimestamp(now),
			TransmitTime:  ntp.TimeToTimestamp(now),
		}
		conn.WriteToUDP(resp.Encode(), raddr)
	}
}
