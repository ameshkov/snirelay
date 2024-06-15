package config

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/netip"
	"os"

	"github.com/ameshkov/snirelay/internal/dnssrv"
)

// DNS represents the DNS server section of the configuration file.
type DNS struct {
	// ListenAddr is the address where the DNS server will listen to incoming
	// requests. Must be specified.
	ListenAddr string `yaml:"listen-addr"`

	// RedirectAddrV4 is the IPv4 address where the DNS server will re-route
	// type=A queries for domains listed in DomainRules. Must be specified.
	RedirectAddrV4 string `yaml:"redirect-addr-v4"`

	// RedirectAddrV6 is the IPv4 address where the DNS server will re-route
	// type=AAAA queries for domains listed in DomainRules. If not specified,
	// the DNS server will respond with empty NOERROR to AAAA queries.
	RedirectAddrV6 string `yaml:"redirect-addr-v6"`

	// PlainPort is the port for plain DNS server. Optional, if not specified,
	// the plain DNS server will not be started.
	PlainPort int `yaml:"plain-port"`

	// TLSPort is the port for DNS-over-TLS server. Optional, if not specified,
	// the plain DNS-over-TLS server will not be started.
	TLSPort int `yaml:"tls-port"`

	// HTTPSPort is the port for DNS-over-HTTPS server. Optional, if not
	// specified, the plain DNS-over-HTTPS server will not be started.
	HTTPSPort int `yaml:"https-port"`

	// QUICPort is the port for DNS-over-QUIC server. Optional, if not
	// specified, the plain DNS-over-QUIC server will not be started.
	QUICPort int `yaml:"quic-port"`

	// UpstreamAddr is the address of the upstream DNS server. This server will
	// be used for queries that shouldn't be re-routed. Must be specified.
	UpstreamAddr string `yaml:"upstream-addr"`

	// RateLimit is the maximum number of requests per second for a plain DNS
	// server. If 0 or not specified, there will be no rate limit.
	RateLimit int `yaml:"rate-limit"`

	// RateLimitAllowlist is a list of IP addresses excluded from rate limiting.
	RateLimitAllowlist []string `yaml:"rate-limit-allowlist"`

	// TLSCertPath is the path to the TLS certificate. It is only required if
	// one of the following properties are specified: TLSPort, HTTPSPort,
	// QUICPort.
	TLSCertPath string `yaml:"tls-cert-path"`

	// TLSKeyPath is the path to the TLS private key. It is only required if
	// one of the following properties are specified: TLSPort, HTTPSPort,
	// QUICPort.
	TLSKeyPath string `yaml:"tls-key-path"`
}

// ToDNSConfig transforms the configuration to the internal dnssrv.Config.
// Note that this method can return nil if DNS section was not specified in the
// configuration.
func (f *File) ToDNSConfig() (dnsCfg *dnssrv.Config, err error) {
	if f.DNS == nil {
		return nil, nil
	}

	dnsCfg = &dnssrv.Config{
		RateLimit: f.DNS.RateLimit,
	}

	for _, ip := range f.DNS.RateLimitAllowlist {
		addr, addrErr := netip.ParseAddr(ip)
		if addrErr != nil {
			return nil, fmt.Errorf("invalid address in rate limit allowlist: %s", addrErr)
		}

		dnsCfg.RateLimitAllowlist = append(dnsCfg.RateLimitAllowlist, addr)
	}

	listenIP := net.ParseIP(f.DNS.ListenAddr)
	if listenIP != nil {
		return nil, fmt.Errorf("failed to parse %s", f.DNS.ListenAddr)
	}

	if f.DNS.PlainPort > 0 {
		dnsCfg.TCPAddr = &net.TCPAddr{IP: listenIP, Port: f.DNS.PlainPort}
		dnsCfg.UDPAddr = &net.UDPAddr{IP: listenIP, Port: f.DNS.PlainPort}
	}

	if f.DNS.TLSPort > 0 {
		dnsCfg.TLSAddr = &net.TCPAddr{IP: listenIP, Port: f.DNS.TLSPort}
	}

	if f.DNS.HTTPSPort > 0 {
		dnsCfg.HTTPSAddr = &net.TCPAddr{IP: listenIP, Port: f.DNS.HTTPSPort}
	}

	if f.DNS.QUICPort > 0 {
		dnsCfg.QUICAddr = &net.UDPAddr{IP: listenIP, Port: f.DNS.QUICPort}
	}

	if f.DNS.RedirectAddrV4 != "" {
		dnsCfg.RedirectAddrIPv4 = net.ParseIP(f.DNS.RedirectAddrV4)
		if dnsCfg.RedirectAddrIPv4 == nil {
			return nil, fmt.Errorf("failed to parse redirect-addr-v4: %s", f.DNS.RedirectAddrV4)
		}
	}

	if f.DNS.RedirectAddrV6 != "" {
		dnsCfg.RedirectAddrIPv6 = net.ParseIP(f.DNS.RedirectAddrV6)
		if dnsCfg.RedirectAddrIPv6 == nil {
			return nil, fmt.Errorf("failed to parse redirect-addr-v6: %s", f.DNS.RedirectAddrV6)
		}
	}

	if f.DNS.TLSCertPath != "" && f.DNS.TLSKeyPath != "" {
		cert, tlsErr := loadX509KeyPair(f.DNS.TLSCertPath, f.DNS.TLSKeyPath)
		if tlsErr != nil {
			return nil, fmt.Errorf("failed to load TLS configuration: %s", tlsErr)
		}

		dnsCfg.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}
	}

	for k, v := range f.DomainRules {
		switch v {
		case actionRelay:
			dnsCfg.RedirectDomains = append(dnsCfg.RedirectDomains, k)
		default:
			return nil, fmt.Errorf("invalid relay rule: %s", v)
		}
	}

	return dnsCfg, nil
}

// loadX509KeyPair reads and parses a public/private key pair from a pair of
// files.  The files must contain PEM encoded data.  The certificate file may
// contain intermediate certificates following the leaf certificate to form a
// certificate chain.  On successful return, Certificate.Leaf will be nil
// because the parsed form of the certificate is not retained.
func loadX509KeyPair(certFile, keyFile string) (crt tls.Certificate, err error) {
	// #nosec G304 -- Trust the file path that is given in the configuration.
	certPEMBlock, err := os.ReadFile(certFile)
	if err != nil {
		return tls.Certificate{}, err
	}

	// #nosec G304 -- Trust the file path that is given in the configuration.
	keyPEMBlock, err := os.ReadFile(keyFile)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.X509KeyPair(certPEMBlock, keyPEMBlock)
}
