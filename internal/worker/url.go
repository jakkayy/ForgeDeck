package worker

import (
	"fmt"
	"net"
	"net/url"
)

// ValidateControlPlaneURL requires HTTPS so the credential is never sent in
// clear text, except to a loopback address or when allowInsecure is set.
func ValidateControlPlaneURL(raw string, allowInsecure bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return fmt.Errorf("control plane URL %q must look like https://forgedeck.example.com", raw)
	}
	if u.Scheme == "https" || allowInsecure || isLoopback(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("control plane URL %q is not HTTPS; use https or pass --allow-insecure for a trusted network", raw)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
