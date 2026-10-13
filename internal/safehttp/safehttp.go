// Package safehttp builds HTTP clients for user-supplied URLs (self-hosted
// GitLab, health checks) that cannot be pointed at ForgeDeck's own network.
// Addresses are checked when connecting, after DNS resolution, so a hostname
// that later resolves to an internal address (DNS rebinding) is still blocked.
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
	// AllowPrivate permits RFC 1918 / unique-local addresses, for operators
	// whose GitLab or registry lives on an internal network.
	AllowPrivate bool
	// AllowLoopback permits 127.0.0.0/8 and ::1. Tests only.
	AllowLoopback bool
}

// ErrBlockedDestination is returned when a URL resolves to a forbidden address.
var ErrBlockedDestination = errors.New("destination address is not allowed")

// NewClient returns a client that refuses blocked destinations and does not
// follow redirects (a redirect could lead somewhere the policy forbids).
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
	// Everything else: link-local (cloud metadata at 169.254.169.254),
	// unspecified, multicast, and anything not explicitly allowed.
	return fmt.Errorf("%w: %s", ErrBlockedDestination, ip)
}
