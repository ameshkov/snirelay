package relay_test

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/AdguardTeam/golibs/log"
	"github.com/AdguardTeam/golibs/netutil"
	"github.com/ameshkov/snirelay/internal/relay"
	"github.com/stretchr/testify/require"
	"github.com/things-go/go-socks5"
)

// TODO(ameshkov): Run a local HTTP bin instead of using remote.
func TestNewServer(t *testing.T) {
	testCases := []struct {
		name            string
		url             string
		redirectDomains []string
		plainHTTP       bool
		proxy           bool
		expectedStatus  int
		expectedNetErr  bool
	}{{
		name:            "plain_http_status_200",
		url:             "http://httpbin.agrd.dev/status/200",
		redirectDomains: []string{"httpbin.agrd.dev"},
		plainHTTP:       true,
		expectedStatus:  http.StatusOK,
	}, {
		name:            "plain_http_status_200_via_proxy",
		url:             "http://httpbin.agrd.dev/status/200",
		redirectDomains: []string{"httpbin.agrd.dev"},
		plainHTTP:       true,
		proxy:           true,
		expectedStatus:  http.StatusOK,
	}, {
		name:            "https_status_200",
		url:             "https://httpbin.agrd.dev/status/200",
		redirectDomains: []string{"httpbin.agrd.dev"},
		plainHTTP:       false,
		expectedStatus:  http.StatusOK,
	}, {
		name:            "https_status_200_via_proxy",
		url:             "https://httpbin.agrd.dev/status/200",
		redirectDomains: []string{"*.agrd.dev"},
		plainHTTP:       false,
		proxy:           true,
		expectedStatus:  http.StatusOK,
	}, {
		name:            "plain_http_not_redirected",
		url:             "http://httpbin.agrd.dev/status/200",
		redirectDomains: []string{"example.org"},
		plainHTTP:       true,
		expectedNetErr:  true,
	}, {
		name:            "https_not_redirected",
		url:             "https://httpbin.agrd.dev/status/200",
		redirectDomains: []string{"example.net"},
		plainHTTP:       false,
		expectedNetErr:  true,
	}}

	// SOCKS proxy is required for tests that use Socks.
	socksListener, socksErr := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, socksErr)
	defer log.OnCloserError(socksListener, log.DEBUG)

	socksServer := socks5.NewServer()
	go func() {
		_ = socksServer.Serve(socksListener)
	}()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var proxyURL *url.URL
			if tc.proxy {
				proxyURL = &url.URL{
					Scheme: "socks5",
					Host:   socksListener.Addr().String(),
				}
			}

			cfg := &relay.Config{
				ListenAddr:      netutil.IPv4Localhost(),
				ListenPort:      0,
				ListenPortTLS:   0,
				ProxyURL:        proxyURL,
				RedirectDomains: tc.redirectDomains,
			}

			r, err := relay.NewServer(cfg)
			require.NoError(t, err)

			err = r.Start()
			require.NoError(t, err)

			defer log.OnCloserError(r, log.ERROR)

			var addr net.Addr
			if tc.plainHTTP {
				addr = r.AddrPlain()
			} else {
				addr = r.AddrTLS()
			}

			client := http.Client{
				Transport: &http.Transport{
					DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
						return net.Dial("tcp", addr.String())
					},
				},
			}

			req, err := http.NewRequest(http.MethodGet, tc.url, nil)
			require.NoError(t, err)

			resp, err := client.Do(req)

			if tc.expectedNetErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)

			defer log.OnCloserError(resp.Body, log.ERROR)

			require.Equal(t, tc.expectedStatus, resp.StatusCode)
		})
	}
}
