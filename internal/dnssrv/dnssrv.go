// Package dnssrv is responsible for DNS server.
package dnssrv

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/AdguardTeam/golibs/container"
	"github.com/AdguardTeam/golibs/log"
	"github.com/IGLOU-EU/go-wildcard"
	"github.com/ameshkov/snirelay/internal/metrics"
	"github.com/miekg/dns"
)

const (
	defaultTTL             = 300
	defaultCacheSizeBytes  = 16 * 1024
	ratelimitSubnetLenIPv4 = 24
	ratelimitSubnetLenIPv6 = 56
)

// Server is the DNS server that is able to re-route domains to the SNI relay.
type Server struct {
	proxy            *proxy.Proxy
	redirectDomains  []string
	redirectAddrIPv4 net.IP
	redirectAddrIPv6 net.IP
}

// New creates a new DNS server with the specified configuration.
func New(config *Config) (srv *Server, err error) {
	proxyCfg := &proxy.Config{}

	proxyCfg.Ratelimit = config.RateLimit
	proxyCfg.RatelimitSubnetLenIPv4 = ratelimitSubnetLenIPv4
	proxyCfg.RatelimitSubnetLenIPv6 = ratelimitSubnetLenIPv6
	proxyCfg.RatelimitWhitelist = config.RateLimitAllowlist

	proxyCfg.CacheEnabled = true
	proxyCfg.CacheSizeBytes = defaultCacheSizeBytes

	proxyCfg.TLSConfig = config.TLSConfig

	proxyCfg.UpstreamConfig = &proxy.UpstreamConfig{
		Upstreams:                []upstream.Upstream{config.Upstream},
		DomainReservedUpstreams:  map[string][]upstream.Upstream{},
		SpecifiedDomainUpstreams: map[string][]upstream.Upstream{},
		SubdomainExclusions:      container.NewMapSet[string](),
	}

	if config.TCPAddr != nil {
		proxyCfg.TCPListenAddr = append(proxyCfg.TCPListenAddr, config.TCPAddr)
	}

	if config.UDPAddr != nil {
		proxyCfg.UDPListenAddr = append(proxyCfg.UDPListenAddr, config.UDPAddr)
	}

	if config.TLSAddr != nil {
		proxyCfg.TLSListenAddr = append(proxyCfg.TLSListenAddr, config.TLSAddr)
	}

	if config.HTTPSAddr != nil {
		proxyCfg.HTTPSListenAddr = append(proxyCfg.HTTPSListenAddr, config.HTTPSAddr)
	}

	if config.QUICAddr != nil {
		proxyCfg.QUICListenAddr = append(proxyCfg.QUICListenAddr, config.QUICAddr)
	}

	srv = &Server{
		redirectDomains:  config.RedirectDomains,
		redirectAddrIPv4: config.RedirectAddrIPv4,
		redirectAddrIPv6: config.RedirectAddrIPv6,
	}

	proxyCfg.RequestHandler = srv.requestHandler

	srv.proxy, err = proxy.New(proxyCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new DNS proxy: %w", err)
	}

	return srv, nil
}

// Start starts the DNS server.
func (s *Server) Start() (err error) {
	return s.proxy.Start(context.Background())
}

// Shutdown stops the DNS server.
func (s *Server) Shutdown(ctx context.Context) (err error) {
	return s.proxy.Shutdown(ctx)
}

// Addr returns the address the proxy listens to for the specified DNS protocol.
func (s *Server) Addr(proto proxy.Proto) (addr net.Addr) {
	return s.proxy.Addr(proto)
}

// requestHandler handles DNS queries and makes a decision based on what
// domain is requested.
func (s *Server) requestHandler(_ *proxy.Proxy, ctx *proxy.DNSContext) (err error) {
	if ctx.Req == nil || len(ctx.Req.Question) != 1 {
		// Invalid request, ignore it immediately.
		return nil
	}

	resp := s.overrideResp(ctx)
	if resp != nil {
		ctx.Res = resp

		return nil
	}

	return s.proxy.Resolve(ctx)
}

// overrideResp checks if it is necessary to override the response. If it is,
// returns the overridden response. Otherwise, returns nil.
func (s *Server) overrideResp(ctx *proxy.DNSContext) (resp *dns.Msg) {
	qHost := ctx.Req.Question[0].Name
	hostname := strings.TrimRight(qHost, ".")
	reqType := ctx.Req.Question[0].Qtype

	log.Debug("[%d] %s %s", ctx.RequestID, dns.Type(reqType), hostname)

	redirect := s.shouldRedirect(hostname)

	redirectLabel := "0"
	if redirect {
		redirectLabel = "1"
	}
	metrics.QueriesTotal.WithLabelValues(string(ctx.Proto), redirectLabel).Inc()

	if !redirect {
		log.Debug("[%d] The domain should not be redirected", ctx.RequestID)

		return nil
	}

	if reqType != dns.TypeA && reqType != dns.TypeAAAA && reqType != dns.TypeHTTPS {
		log.Debug("[%d] This type of requests should not be redirected", ctx.RequestID)

		return nil
	}

	resp = new(dns.Msg)
	resp.SetReply(ctx.Req)
	resp.Compress = true

	switch {
	case reqType == dns.TypeA && s.redirectAddrIPv4 != nil:
		log.Debug("[%d] Override IPv4 to %s", ctx.RequestID, s.redirectAddrIPv4)

		resp.Answer = []dns.RR{
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   qHost,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    defaultTTL,
				},
				A: s.redirectAddrIPv4,
			},
		}
	case reqType == dns.TypeAAAA && s.redirectAddrIPv6 != nil:
		log.Debug("[%d] Override IPv6 to %s", ctx.RequestID, s.redirectAddrIPv6)

		resp.Answer = []dns.RR{
			&dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   qHost,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    defaultTTL,
				},
				AAAA: s.redirectAddrIPv6,
			},
		}
	default:
		log.Debug("[%d] Return empty NOERROR response", ctx.RequestID)
	}

	return resp
}

// shouldRedirect checks if the hostname needs to be redirected.
func (s *Server) shouldRedirect(hostname string) (ok bool) {
	for _, pattern := range s.redirectDomains {
		if wildcard.MatchSimple(pattern, hostname) {
			return true
		}
	}

	return false
}
