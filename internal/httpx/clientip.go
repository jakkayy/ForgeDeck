package httpx

import (
	"context"
	"net/http"
	"net/netip"
	"strings"
)

type clientIPKey struct{}

// TrustedProxies makes ClientIP report the real client when ForgeDeck runs
// behind a reverse proxy. Without it every request would appear to come from
// the proxy, and any limit per client address would be one shared bucket that
// a single caller can empty for everybody.
//
// The X-Forwarded-For header is believed only when the TCP peer is in
// proxies, because anyone else can send it with any value. Then the header is
// read from the right, the way proxies append to it: the first address that
// is not itself a trusted proxy is the client. Addresses further left were
// supplied by the client and are ignored. With no trusted proxies this
// middleware changes nothing.
func TrustedProxies(proxies []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if len(proxies) == 0 {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer := peerIP(r)
			client := peer
			if trusted(proxies, peer) {
				client = fromForwardedFor(proxies, r.Header.Values("X-Forwarded-For"), peer)
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientIPKey{}, client)))
		})
	}
}

func trusted(proxies []netip.Prefix, address string) bool {
	addr, err := netip.ParseAddr(address)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range proxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// fromForwardedFor returns the right-most address in the header that is not a
// trusted proxy, or fallback when the header is missing or has no such address.
func fromForwardedFor(proxies []netip.Prefix, values []string, fallback string) string {
	var hops []string
	for _, value := range values {
		for _, hop := range strings.Split(value, ",") {
			hops = append(hops, strings.TrimSpace(hop))
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(hops[i])
		if err != nil {
			return fallback // a malformed hop: do not guess
		}
		if !trusted(proxies, addr.String()) {
			return addr.Unmap().String()
		}
	}
	return fallback
}
