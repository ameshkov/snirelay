package relay_test

import (
	"context"
	"net"
	"net/http"
	"testing"

	"bit.int.agrd.dev/relay/internal/relay"
	"github.com/AdguardTeam/golibs/log"
	"github.com/stretchr/testify/require"
)

func TestNewServer_plainHTTP(t *testing.T) {
	testCases := []struct {
		name           string
		url            string
		plainHTTP      bool
		expectedStatus int
	}{{
		name:           "plain_http_status_200",
		url:            "http://httpbin.agrd.workers.dev/status/200",
		plainHTTP:      true,
		expectedStatus: http.StatusOK,
	}, {
		name:           "https_status_200",
		url:            "https://httpbin.agrd.workers.dev/status/200",
		plainHTTP:      false,
		expectedStatus: http.StatusOK,
	}}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := relay.NewServer("127.0.0.1", 0, 0, nil)
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
