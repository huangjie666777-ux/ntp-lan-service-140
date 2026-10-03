// Command client queries an NTP server once and prints the decoded reply.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"time"
)

const ntpEpochOffset = 2208988800

func main() {
	server := flag.String("server", "127.0.0.1:10123", "NTP server address")
	flag.Parse()

	raddr, err := net.ResolveUDPAddr("udp4", *server)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := net.DialUDP("udp4", nil, raddr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	req := make([]byte, 48)
	req[0] = 4<<3 | 3 // VN=4, mode=3
	t1 := time.Now()
	sec := uint64(t1.Unix()) + ntpEpochOffset
	frac := uint64(t1.Nanosecond()) * (1 << 32) / 1e9
	binary.BigEndian.PutUint32(req[40:], uint32(sec))
	binary.BigEndian.PutUint32(req[44:], uint32(frac))

	if _, err := conn.Write(req); err != nil {
		log.Fatal(err)
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		log.Fatal(err)
	}
	t4 := time.Now()
	if n < 48 {
		log.Fatalf("short reply: %d bytes", n)
	}
	b := buf[:48]
	li, vn, mode := b[0]>>6, (b[0]>>3)&7, b[0]&7
	stratum := b[1]
	toTime := func(off int) time.Time {
		s := binary.BigEndian.Uint32(b[off:])
		f := binary.BigEndian.Uint32(b[off+4:])
		return time.Unix(int64(s)-ntpEpochOffset, int64(f)*1e9/(1<<32))
	}
	t2, t3 := toTime(32), toTime(40)
	fmt.Printf("LI=%d VN=%d mode=%d stratum=%d\n", li, vn, mode, stratum)
	if li == 3 || stratum == 16 {
		fmt.Println("server is NOT synchronized (LI=3/stratum=16)")
		return
	}
	offset := (t2.Sub(t1) + t3.Sub(t4)) / 2
	delay := t4.Sub(t1) - t3.Sub(t2)
	fmt.Printf("offset=%v delay=%v server_time=%s\n", offset, delay, t3.Format(time.RFC3339Nano))
}
