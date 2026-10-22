// Package safehttp builds HTTP clients for user-supplied URLs that cannot reach internal addresses (checked after DNS)
package safehttp

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// Policy chooses which destinations are reachable besides public addresses.
type Policy struct {
	// AllowPrivate permits RFC 1918 / unique-local addresses
	AllowPrivate bool
	// AllowLoopback permits 127.0.0.0/8 and ::1. Tests only.
	AllowLoopback bool
}

// ErrBlockedDestination is returned when a URL resolves to a forbidden address.
var ErrBlockedDestination = errors.New("destination address is not allowed")

// NewClient returns a client that refuses blocked destinations and never follows redirects
func NewClient(policy Policy, timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			return policy.check(address)
		},
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	transport.Proxy = nil // a proxy would hide the real destination from the check
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (p Policy) check(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBlockedDestination, host)
	}
	ip = ip.Unmap()
	switch {
	case ip.IsLoopback():
		if p.AllowLoopback {
			return nil
		}
	case ip.IsPrivate():
		if p.AllowPrivate {
			return nil
		}
	case ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast():
		return nil
	}
	// link-local (cloud metadata), unspecified, multicast and the rest stay blocked
	return fmt.Errorf("%w: %s", ErrBlockedDestination, ip)
}
