package httpx

import (
	"context"
	"net/http"
	"net/netip"
	"strings"
)

type clientIPKey struct{}

// TrustedProxies makes ClientIP honor X-Forwarded-For (read from the right) only from trusted peers
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

// fromForwardedFor returns the right-most address that is not a trusted proxy, else fallback
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
