// Command client sends a mode-3 request and prints the decoded response,
// including whether the server claims to be synchronized.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/huangjie666777-ux/ntp-lan-service-140/internal/ntp"
)

func main() {
	server := flag.String("server", "127.0.0.1:1230", "NTP server host:port")
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
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))

	req := &ntp.Packet{Version: ntp.Version, Mode: ntp.ModeClient, TransmitTime: ntp.TimeToTimestamp(time.Now())}
	t1 := time.Now()
	if _, err := conn.Write(req.Encode()); err != nil {
		log.Fatal(err)
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		log.Fatal(err)
	}
	t4 := time.Now()
	resp, err := ntp.DecodePacket(buf[:n])
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("LI=%d version=%d mode=%d stratum=%d\n", resp.LI, resp.Version, resp.Mode, resp.Stratum)
	if resp.LI == 3 || resp.Stratum >= 16 {
		fmt.Println("server is NOT synchronized (LI=3 / stratum 16)")
		return
	}
	t2, t3 := resp.ReceiveTime.ToTime(), resp.TransmitTime.ToTime()
	offset := (t2.Sub(t1) + t3.Sub(t4)) / 2
	delay := t4.Sub(t1) - t3.Sub(t2)
	fmt.Printf("offset=%s delay=%s serverTime=%s\n", offset, delay, t3.Format(time.RFC3339Nano))
}
