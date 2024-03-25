package relay_test

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"testing"

	"bit.int.agrd.dev/relay/internal/relay"
	"github.com/AdguardTeam/golibs/log"
	"github.com/stretchr/testify/require"
	"github.com/things-go/go-socks5"
)

func TestNewServer(t *testing.T) {
	testCases := []struct {
		name           string
		url            string
		plainHTTP      bool
		proxy          bool
		expectedStatus int
	}{{
		name:           "plain_http_status_200",
		url:            "http://httpbin.agrd.workers.dev/status/200",
		plainHTTP:      true,
		expectedStatus: http.StatusOK,
	}, {
		name:           "plain_http_status_200_via_proxy",
		url:            "http://httpbin.agrd.workers.dev/status/200",
		plainHTTP:      true,
		proxy:          true,
		expectedStatus: http.StatusOK,
	}, {
		name:           "https_status_200",
		url:            "https://httpbin.agrd.workers.dev/status/200",
		plainHTTP:      false,
		expectedStatus: http.StatusOK,
	}, {
		name:           "https_status_200_via_proxy",
		url:            "https://httpbin.agrd.workers.dev/status/200",
		plainHTTP:      false,
		proxy:          true,
		expectedStatus: http.StatusOK,
	}}

	// It is required for some tests.
	socksListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
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

			r, err := relay.NewServer("127.0.0.1", 0, 0, proxyURL, nil)
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
			require.NoError(t, err)

			defer log.OnCloserError(resp.Body, log.ERROR)

			require.Equal(t, tc.expectedStatus, resp.StatusCode)
		})
	}
}
