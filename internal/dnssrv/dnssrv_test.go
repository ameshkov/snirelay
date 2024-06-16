package dnssrv_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/proxy"
	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/ameshkov/snirelay/internal/dnssrv"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

const tlsServerName = "dns.example.com"
const relayUpstreamAddr = "tls://dns.google"

func TestNew(t *testing.T) {
	testCases := []struct {
		name             string
		reqDomain        string
		redirectDomains  []string
		expectedRedirect bool
		proto            proxy.Proto
	}{{
		name:             "redirect-plain",
		reqDomain:        "example.com",
		redirectDomains:  []string{"*"},
		expectedRedirect: true,
		proto:            proxy.ProtoUDP,
	}, {
		name:             "redirect-https",
		reqDomain:        "example.com",
		redirectDomains:  []string{"*"},
		expectedRedirect: true,
		proto:            proxy.ProtoHTTPS,
	}, {
		name:             "redirect-tls",
		reqDomain:        "example.com",
		redirectDomains:  []string{"*"},
		expectedRedirect: true,
		proto:            proxy.ProtoTLS,
	}, {
		name:             "redirect-quic",
		reqDomain:        "example.com",
		redirectDomains:  []string{"*"},
		expectedRedirect: true,
		proto:            proxy.ProtoQUIC,
	}, {
		name:             "no-redirect",
		reqDomain:        "example.org",
		redirectDomains:  []string{"example.com"},
		expectedRedirect: false,
		proto:            proxy.ProtoUDP,
	}, {
		name:             "no-redirect-tls",
		reqDomain:        "example.org",
		redirectDomains:  []string{"example.com"},
		expectedRedirect: false,
		proto:            proxy.ProtoTLS,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			localAddrUDP := &net.UDPAddr{IP: net.IP{127, 0, 0, 1}, Port: 0}
			localAddrTCP := &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 0}
			redirectIpv4 := "127.0.0.1"

			tlsConfig, caPem := newTLSConfig(t)
			roots := x509.NewCertPool()
			roots.AppendCertsFromPEM(caPem)

			u, err := upstream.AddressToUpstream(relayUpstreamAddr, &upstream.Options{})
			require.NoError(t, err)

			cfg := &dnssrv.Config{
				Upstream:         u,
				RedirectDomains:  tc.redirectDomains,
				RedirectAddrIPv4: net.ParseIP(redirectIpv4),
				TLSConfig:        tlsConfig,
			}

			switch tc.proto {
			case proxy.ProtoUDP, proxy.ProtoTCP:
				cfg.TCPAddr = localAddrTCP
				cfg.UDPAddr = localAddrUDP
			case proxy.ProtoTLS:
				cfg.TLSAddr = localAddrTCP
			case proxy.ProtoHTTPS:
				cfg.HTTPSAddr = localAddrTCP
			case proxy.ProtoQUIC:
				cfg.QUICAddr = localAddrUDP
			}

			srv, err := dnssrv.New(cfg)
			require.NoError(t, err)

			err = srv.Start()
			require.NoError(t, err)

			defer func(srv *dnssrv.Server, ctx context.Context) {
				_ = srv.Shutdown(ctx)
			}(srv, context.Background())

			upstreamAddr := ""
			switch tc.proto {
			case proxy.ProtoUDP:
				upstreamAddr = fmt.Sprintf("%s", srv.Addr(proxy.ProtoUDP))
			case proxy.ProtoTCP:
				upstreamAddr = fmt.Sprintf("%s", srv.Addr(proxy.ProtoTCP))
			case proxy.ProtoHTTPS:
				upstreamAddr = fmt.Sprintf("https://%s/dns-query", srv.Addr(proxy.ProtoHTTPS))
			case proxy.ProtoTLS:
				upstreamAddr = fmt.Sprintf("tls://%s", srv.Addr(proxy.ProtoTLS))
			case proxy.ProtoQUIC:
				upstreamAddr = fmt.Sprintf("quic://%s", srv.Addr(proxy.ProtoQUIC))
			}

			testUpstream, err := upstream.AddressToUpstream(
				upstreamAddr,
				&upstream.Options{RootCAs: roots},
			)
			require.NoError(t, err)

			for _, reqType := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeHTTPS} {
				req := &dns.Msg{}
				req.Question = []dns.Question{
					{Name: dns.Fqdn(tc.reqDomain), Qtype: reqType, Qclass: dns.ClassINET},
				}

				resp, exchErr := testUpstream.Exchange(req)
				require.NoError(t, exchErr)
				require.NotNil(t, resp)
				require.Equal(t, dns.RcodeSuccess, resp.Rcode)

				if tc.expectedRedirect {
					if reqType == dns.TypeA {
						require.Len(t, resp.Answer, 1)
						a, ok := resp.Answer[0].(*dns.A)
						require.True(t, ok)
						require.Equal(t, redirectIpv4, a.A.String())
					} else {
						require.Empty(t, req.Answer)
					}
				} else {
					if reqType == dns.TypeA {
						var ips []string
						for _, a := range resp.Answer {
							ips = append(ips, a.(*dns.A).A.String())
						}

						require.NotEmpty(t, ips)
						require.NotContains(t, ips, redirectIpv4)
					}
				}
			}
		})
	}
}

func newTLSConfig(t *testing.T) (conf *tls.Config, certPem []byte) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	require.NoError(t, err)

	notBefore := time.Now()
	notAfter := notBefore.Add(5 * 365 * time.Hour * 24)

	keyUsage := x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign
	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               pkix.Name{Organization: []string{"AdGuard Tests"}},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              keyUsage,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{tlsServerName},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		&privateKey.PublicKey,
		privateKey,
	)
	require.NoError(t, err)

	certPem = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})
	keyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	cert, err := tls.X509KeyPair(certPem, keyPem)
	require.NoError(t, err)

	return &tls.Config{Certificates: []tls.Certificate{cert}, ServerName: tlsServerName}, certPem
}
