// Command upstream is a local test NTP server with a configurable fake
// offset and stratum, used to demo the LAN time service.
package main

import (
	"encoding/binary"
	"flag"
	"log"
	"net"
	"time"
)

const ntpEpochOffset = 2208988800

func ntpNow(offset time.Duration) (uint32, uint32) {
	t := time.Now().Add(offset)
	sec := uint64(t.Unix()) + ntpEpochOffset
	frac := uint64(t.Nanosecond()) * (1 << 32) / 1e9
	return uint32(sec), uint32(frac)
}

func main() {
	listen := flag.String("listen", "127.0.0.1:11230", "UDP listen address")
	offset := flag.Duration("offset", 0, "fake clock offset to serve")
	stratum := flag.Int("stratum", 1, "stratum to report")
	li := flag.Int("li", 0, "leap indicator to report")
	flag.Parse()

	addr, err := net.ResolveUDPAddr("udp4", *listen)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("upstream on %s offset=%v stratum=%d li=%d", *listen, *offset, *stratum, *li)

	buf := make([]byte, 512)
	for {
		n, raddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if n < 48 || buf[0]&0x7 != 3 {
			continue
		}
		resp := make([]byte, 48)
		resp[0] = byte(*li)<<6 | 4<<3 | 4 // LI, VN=4, mode=4
		resp[1] = byte(*stratum)
		copy(resp[24:32], buf[40:48]) // originate = client transmit
		sec, frac := ntpNow(*offset)
		binary.BigEndian.PutUint32(resp[32:], sec)
		binary.BigEndian.PutUint32(resp[36:], frac)
		sec, frac = ntpNow(*offset)
		binary.BigEndian.PutUint32(resp[40:], sec)
		binary.BigEndian.PutUint32(resp[44:], frac)
		_, _ = conn.WriteToUDP(resp, raddr)
	}
}
