package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/huangjie666777-ux/ntp-lan-service-140/internal/ntp"
)

func main() {
	var upstreams multiFlag
	udpAddr := flag.String("udp", "127.0.0.1:1230", "UDP listen address for NTP server")
	httpAddr := flag.String("http", "127.0.0.1:8080", "HTTP listen address for status API")
	poll := flag.Duration("poll", 5*time.Second, "upstream poll interval")
	timeout := flag.Duration("timeout", 2*time.Second, "per-request UDP timeout")
	validity := flag.Duration("validity", 30*time.Second, "sample validity window (monotonic)")
	threshold := flag.Duration("threshold", 500*time.Millisecond, "consensus threshold around median offset")
	flag.Var(&upstreams, "upstream", "upstream IPv4 host:port (repeatable, exactly 3 required)")
	flag.Parse()

	if len(upstreams) != 3 {
		log.Fatal("exactly 3 -upstream IPv4 addresses are required")
	}
	for _, u := range upstreams {
		if err := ntp.CheckIPv4HostPort(u); err != nil {
			log.Fatalf("invalid upstream %q: %v", u, err)
		}
	}

	clock := &ntp.Clock{}
	sources := make([]*ntp.Source, len(upstreams))
	for i, u := range upstreams {
		sources[i] = ntp.NewSource(u)
	}
	poller := ntp.NewPoller(sources, *poll, *timeout, *validity, *threshold, clock)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go poller.Run(ctx)

	srv := ntp.NewServer(clock)
	if err := srv.Listen(*udpAddr); err != nil {
		log.Fatalf("udp listen: %v", err)
	}
	defer srv.Close()
	go srv.Serve(ctx)

	handler := ntp.NewStatusHandler(sources, poller, clock, *validity)
	httpSrv := &http.Server{Addr: *httpAddr, Handler: handler.Router()}
	go func() {
		log.Printf("http status on %s", *httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()
	log.Printf("ntp server on %s, upstreams: %s", *udpAddr, strings.Join(upstreams, ", "))

	<-ctx.Done()
	log.Println("shutting down")
	srv.Close() // release UDP port, stops poller replies
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	httpSrv.Shutdown(shutdownCtx)
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
