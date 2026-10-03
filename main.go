package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfgPath := flag.String("config", "config.json", "path to JSON config")
	flag.Parse()

	cfg, err := LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	clock := &VirtualClock{}
	poller := NewPoller(cfg, clock)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go poller.Run(ctx)

	udp := NewUDPServer(cfg.UDPListen, clock)
	go func() {
		if err := udp.Run(ctx); err != nil {
			log.Fatalf("udp server: %v", err)
		}
	}()

	httpSrv := &http.Server{Addr: cfg.HTTPListen, Handler: NewHTTPHandler(poller, clock, cfg)}
	go func() {
		log.Printf("http: status on %s/status", cfg.HTTPListen)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	shCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shCtx) // releases HTTP port; UDP closes via ctx
}
