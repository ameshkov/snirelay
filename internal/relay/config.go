package relay

import (
	"net/netip"
	"net/url"
)

// Config represents the SNI relay server configuration.
type Config struct {
	// ListenAddr is the address the SNI relay server will listen to.
	ListenAddr netip.Addr

	// ListenPort is the port the SNI relay expects to receive plain HTTP
	// requests to.
	ListenPort uint16

	// ListenPortTLS is the port the SNI relay expects to receive HTTPS requests
	// to.
	ListenPortTLS uint16

	// ProxyURL is the proxy server address (optional).
	ProxyURL *url.URL

	// RedirectDomains is a list of wildcards the relay server can reroute.
	// If the incoming connection is not from this list, the connection will
	// not be accepted.
	RedirectDomains []string
}
