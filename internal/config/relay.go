package config

import (
	"fmt"
	"net/netip"
	"net/url"

	"github.com/ameshkov/snirelay/internal/relay"
)

// Relay represents the SNI relay server section of the configuration file.
type Relay struct {
	// ListenAddr is the address where the Relay server will listen to incoming
	// connections.
	ListenAddr string `yaml:"listen-addr"`

	// HTTPPort is the port where relay will expect to receive plain HTTP
	// connections.
	HTTPPort uint16 `yaml:"http-port"`

	// HTTPSPort is the port where relay will expect to receive HTTPS
	// connections.
	HTTPSPort uint16 `yaml:"https-port"`

	// ProxyURL is the optional port for upstream connections by the relay.
	// Format of the URL: [protocol://username:password@]host[:port]
	ProxyURL string `yaml:"proxy-url"`
}

// ToRelayConfig transforms the configuration to the internal relay.Config.
func (f *File) ToRelayConfig() (relayCfg *relay.Config, err error) {
	if f.Relay == nil {
		return nil, fmt.Errorf("relay config is empty")
	}

	relayCfg = &relay.Config{
		ListenPort:    f.Relay.HTTPPort,
		ListenPortTLS: f.Relay.HTTPSPort,
	}

	relayCfg.ListenAddr, err = netip.ParseAddr(f.Relay.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("parse relay listen addr: %w", err)
	}

	if f.Relay.ProxyURL != "" {
		relayCfg.ProxyURL, err = url.Parse(f.Relay.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("parse relay proxy url: %w", err)
		}
	}

	for k, v := range f.DomainRules {
		switch v {
		case actionRelay:
			relayCfg.RedirectDomains = append(relayCfg.RedirectDomains, k)
		default:
			return nil, fmt.Errorf("invalid relay rule: %s", v)
		}
	}

	return relayCfg, nil
}
