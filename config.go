package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config holds all runtime settings for the LAN NTP service.
type Config struct {
	Upstreams      []UpstreamConfig `json:"upstreams"`
	UDPListen      string           `json:"udp_listen"`
	HTTPListen     string           `json:"http_listen"`
	PollInterval   Duration         `json:"poll_interval"`
	PollTimeout    Duration         `json:"poll_timeout"`
	SampleMaxAge   Duration         `json:"sample_max_age"`
	AgreeThreshold Duration         `json:"agree_threshold"`
}

// UpstreamConfig describes one IPv4 NTP upstream, in config order.
type UpstreamConfig struct {
	Name    string `json:"name"`
	Address string `json:"address"` // host:port, IPv4 only
}

// Duration is a JSON-friendly time.Duration ("500ms", "10s").
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if len(cfg.Upstreams) != 3 {
		return nil, fmt.Errorf("exactly 3 upstreams required, got %d", len(cfg.Upstreams))
	}
	for i, u := range cfg.Upstreams {
		if u.Address == "" {
			return nil, fmt.Errorf("upstream %d missing address", i)
		}
	}
	if cfg.UDPListen == "" || cfg.HTTPListen == "" {
		return nil, fmt.Errorf("udp_listen and http_listen are required")
	}
	if cfg.PollInterval.Duration <= 0 || cfg.PollTimeout.Duration <= 0 ||
		cfg.SampleMaxAge.Duration <= 0 || cfg.AgreeThreshold.Duration <= 0 {
		return nil, fmt.Errorf("poll_interval, poll_timeout, sample_max_age, agree_threshold must be > 0")
	}
	return &cfg, nil
}
