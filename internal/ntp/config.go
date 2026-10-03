package ntp

import (
	"fmt"
	"net"
	"strconv"
)

// CheckIPv4HostPort validates a host:port whose host is a literal IPv4 address.
func CheckIPv4HostPort(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("host %q is not an IPv4 address", host)
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("invalid port %q", port)
	}
	return nil
}
