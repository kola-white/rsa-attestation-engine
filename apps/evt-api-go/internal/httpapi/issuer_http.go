package httpapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

var issuerMetadataHTTPClient = issuerHTTPClient()

// Metadata URLs originate in unverified credentials. Check the actual addresses
// we dial, including after redirects, rather than trusting a DNS preflight.
func issuerHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil // An environment proxy would bypass destination checks.
	transport.DialContext = dialPublicIssuer
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

func dialPublicIssuer(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("issuer host has no addresses")
	}
	for _, address := range addresses {
		if !publicIssuerAddress(address) {
			return nil, fmt.Errorf("issuer destination is not public")
		}
	}
	var dialer net.Dialer
	var lastErr error
	for _, address := range addresses {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func publicIssuerAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.Zone() != "" {
		return false
	}
	// Special-use ranges that IsGlobalUnicast still includes. Block shared,
	// documentation, benchmarking, reserved, and IPv4 translation destinations.
	for _, prefix := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24",
		"192.88.99.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
		"64:ff9b::/96", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "2002::/16",
	} {
		if netip.MustParsePrefix(prefix).Contains(address) {
			return false
		}
	}
	return true
}
