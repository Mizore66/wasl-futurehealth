package main

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// validateLocalEndpoint restricts clinical inference and mock notification destinations to explicit loopback HTTP(S).
// DNS names other than localhost are rejected rather than resolved, avoiding DNS rebinding and ambiguous dual-homing.
// validateLocalEndpoint restricts clinical inference destinations to explicit loopback HTTP(S).
// DNS names other than localhost are rejected rather than resolved, avoiding DNS rebinding and ambiguous dual-homing.
func validateLocalEndpoint(raw string) error {
	return validateLoopbackURL(raw, false)
}

// validateLocalCallback permits the local mock consumer's fixed /hook path while applying the same host restrictions.
func validateLocalCallback(raw string) error {
	return validateLoopbackURL(raw, true)
}

func validateLoopbackURL(raw string, allowPath bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return fmt.Errorf("endpoint must be an explicit loopback HTTP(S) URL")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (!allowPath && u.Path != "" && u.Path != "/") {
		return fmt.Errorf("endpoint must be an explicit loopback HTTP(S) URL without credentials, query, fragment or unexpected path")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("endpoint host must be 127.0.0.1, ::1 or localhost")
		}
	}
	return nil
}
