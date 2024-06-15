// Package config is responsible for parsing configuration file.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// actionRelay is the action name for domain-rules.
const actionRelay = "relay"

// File represents a configuration file.
type File struct {
	// DNS is the DNS server section of the configuration file. If not
	// specified, the DNS server will not be started.
	DNS *DNS `yaml:"dns"`

	// Relay is the SNI relay server section of the configuration file. Must be
	// specified.
	Relay *Relay `yaml:"relay"`

	// Prometheus
	Prometheus *Prometheus `yaml:"prometheus"`

	// DomainRules is the map that controls what the snirelay does with the
	// domains. The key of this map is a wildcard and the value is the action.
	// Must be specified.
	//
	// If the domain is not specified in DomainRules, DNS queries for it will
	// be simply proxied to the upstream DNS server and no re-routing occurs.
	// Connections to the relay server for domains that are not listed will not
	// be accepted.
	//
	// If the action is "relay" then the DNS server will respond to A/AAAA
	// queries and re-route traffic to the relay server. HTTPS queries will be
	// suppressed in this case.
	DomainRules map[string]string `yaml:"domain-rules"`
}

// Prometheus represents the prometheus configuration.
type Prometheus struct {
	// Addr is the address where prometheus metrics are exposed.
	Addr string `yaml:"addr"`

	// Port is the port where prometheus metrics will be exposed.
	Port uint16 `yaml:"port"`
}

// Load loads and validates configuration from the specified file.
func Load(path string) (cfg *File, err error) {
	// Ignore G304 here as it's trusted context.
	//nolint:gosec
	b, err := os.ReadFile(path)

	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg = &File{}
	err = yaml.Unmarshal(b, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	err = validate(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to validate config file: %w", err)
	}

	return cfg, nil
}

func validate(cfg *File) (err error) {
	if cfg.Relay == nil {
		return fmt.Errorf("no relay configured")
	}

	if cfg.DomainRules == nil {
		return fmt.Errorf("no domain-rules configured")
	}

	if cfg.DNS != nil {
		if cfg.DNS.ListenAddr == "" {
			return fmt.Errorf("dnssrv.listen-addr is required")
		}

		if cfg.DNS.RedirectAddrV4 == "" {
			return fmt.Errorf("dnssrv.redirect-addr-v4 is required")
		}

		if cfg.DNS.PlainPort == 0 &&
			cfg.DNS.TLSPort == 0 &&
			cfg.DNS.HTTPSPort == 0 &&
			cfg.DNS.QUICPort == 0 {
			return fmt.Errorf("at least one on dnssrv ports must be configured")
		}

		if cfg.DNS.TLSPort > 0 ||
			cfg.DNS.QUICPort > 0 ||
			cfg.DNS.HTTPSPort > 0 {
			if cfg.DNS.TLSCertPath == "" || cfg.DNS.TLSKeyPath == "" {
				return fmt.Errorf("missing tls configuration")
			}
		}
	}

	return nil
}
