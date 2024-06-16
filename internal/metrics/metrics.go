// Package metrics contains definitions of most of the prometheus metrics
// that we use in snirelay.
//
// TODO(ameshkov): consider not using promauto.
//
// TODO(a.garipov): Add more metrics examples.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// constants with the namespace and the subsystem names that we use in our
// prometheus metrics.
const (
	namespace = "snirelay"

	subsystemApp   = "app"
	subsystemDNS   = "dns"
	subsystemRelay = "relay"
)

// QueriesTotal is the total number of DNS queries.
var QueriesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: namespace,
	Subsystem: subsystemDNS,
	Name:      "queries_total",
	Help:      "The total number of DNS queries.",
}, []string{"proto", "redirected"})

// ConnectionsTotal is a gauge with the total number of active connections
// to the service.
var ConnectionsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Namespace: namespace,
	Subsystem: subsystemRelay,
	Name:      "conns_num",
	Help:      "The total number of connections to the SNI relay service.",
}, []string{"servername"})

// BytesReceivedTotal is a counter that measures the number of bytes received
// from a particular remote endpoint.
var BytesReceivedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: namespace,
	Subsystem: subsystemRelay,
	Name:      "bytes_received_total",
	Help:      "The total number of bytes received from the remote endpoint.",
}, []string{"servername"})

// BytesSentTotal is a counter that measures the number of bytes sent
// to a particular remote endpoint.
var BytesSentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: namespace,
	Subsystem: subsystemRelay,
	Name:      "bytes_sent_total",
	Help:      "The total number of bytes sent to the remote endpoint.",
}, []string{"servername"})

// SetUpGauge signals that the server has been started.  Use a function here to
// avoid circular dependencies.
//
// TODO(a.garipov): Consider adding commit time.
func SetUpGauge(version, branch, revision, goVersion string) {
	upGauge := promauto.NewGauge(
		prometheus.GaugeOpts{
			Name:      "up",
			Namespace: namespace,
			Subsystem: subsystemApp,
			Help:      `A metric with a constant '1' value labeled by the build information.`,
			ConstLabels: prometheus.Labels{
				"version":   version,
				"branch":    branch,
				"revision":  revision,
				"goversion": goVersion,
			},
		},
	)

	upGauge.Set(1)
}
