package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

func clientIP(t *testing.T, proxies []string, peer string, forwarded ...string) string {
	t.Helper()
	var prefixes []netip.Prefix
	for _, p := range proxies {
		prefixes = append(prefixes, netip.MustParsePrefix(p))
	}
	var got string
	handler := httpx.TrustedProxies(prefixes)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = httpx.ClientIP(r)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = peer
	for _, value := range forwarded {
		req.Header.Add("X-Forwarded-For", value)
	}
	handler.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestClientIPWithoutTrustedProxiesIsThePeer(t *testing.T) {
	if got := clientIP(t, nil, "203.0.113.9:4444", "198.51.100.1"); got != "203.0.113.9" {
		t.Fatalf("got %q: the header was believed with no trusted proxy", got)
	}
}

func TestForwardedForFromAnUntrustedPeerIsIgnored(t *testing.T) {
	// anyone can send the header; only a trusted proxy's word counts.
	if got := clientIP(t, []string{"10.0.0.0/8"}, "203.0.113.9:4444", "1.2.3.4"); got != "203.0.113.9" {
		t.Fatalf("got %q", got)
	}
}

func TestClientIPIsTheRightmostUntrustedHop(t *testing.T) {
	proxies := []string{"10.0.0.0/8", "192.168.1.5/32"}
	cases := map[string]struct {
		peer      string
		forwarded []string
		want      string
	}{
		"one proxy":                   {"10.0.0.2:1", []string{"198.51.100.7"}, "198.51.100.7"},
		"two proxies":                 {"10.0.0.2:1", []string{"198.51.100.7, 10.0.0.9"}, "198.51.100.7"},
		"a client-supplied left hop":  {"10.0.0.2:1", []string{"6.6.6.6, 198.51.100.7"}, "198.51.100.7"},
		"the header split in two":     {"10.0.0.2:1", []string{"6.6.6.6", "198.51.100.7, 192.168.1.5"}, "198.51.100.7"},
		"no header":                   {"10.0.0.2:1", nil, "10.0.0.2"},
		"every hop is a proxy":        {"10.0.0.2:1", []string{"10.0.0.8, 10.0.0.9"}, "10.0.0.2"},
		"a malformed hop is not used": {"10.0.0.2:1", []string{"198.51.100.7, garbage"}, "10.0.0.2"},
		"ipv6 client":                 {"10.0.0.2:1", []string{"2001:db8::1"}, "2001:db8::1"},
		"ipv4 mapped in ipv6":         {"10.0.0.2:1", []string{"::ffff:198.51.100.7"}, "198.51.100.7"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := clientIP(t, proxies, tc.peer, tc.forwarded...); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
