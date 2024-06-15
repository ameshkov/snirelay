package dnssrv

import (
	"crypto/tls"
	"net"
	"net/netip"

	"github.com/AdguardTeam/dnsproxy/upstream"
)

// Config represents the DNS server configuration.
type Config struct {
	// Upstream is the DNS upstream that will be used for queries that
	// shouldn't be re-routed.
	Upstream upstream.Upstream

	// RedirectAddrIPv4 is the IP address where type=A queries must be
	// redirected for domains that match RedirectDomains.
	RedirectAddrIPv4 net.IP

	// RedirectAddrIPv4 is the IP address where type=AAAA queries must be
	// redirected for domains that match RedirectDomains.
	RedirectAddrIPv6 net.IP

	// RedirectDomains is a list of wildcards for domains that needs to be
	// redirected.
	RedirectDomains []string

	// TCPAddr is the address for the plain DNS TCP server.
	TCPAddr *net.TCPAddr

	// UDPAddr is the address for the plain DNS TCP server.
	UDPAddr *net.UDPAddr

	// TLSAddr is the address for the DoT server.
	TLSAddr *net.TCPAddr

	// HTTPSAddr is the address for the DoH server.
	HTTPSAddr *net.TCPAddr

	// QUICAddr is the address for the DoQ server.
	QUICAddr *net.UDPAddr

	// TLSConfig is TLS configuration for DoT/DoH/DoQ.
	TLSConfig *tls.Config

	// RateLimit is a number of plain DNS queries per second that are allowed.
	RateLimit int

	// RateLimitAllowlist is a list of IP addresses excluded from rate limiting.
	RateLimitAllowlist []netip.Addr
}
