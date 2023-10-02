package relay

import (
	"context"
	"net"
	"sync"
)

// Resolver is a simple DNS resolver with internal cache.
type Resolver struct {
	res *net.Resolver

	cache   map[string][]net.IP
	cacheMu *sync.Mutex
}

// NewResolver creates a new *Resolver instance.
func NewResolver(resolverCache map[string][]net.IP) (r *Resolver) {
	return &Resolver{
		res:     &net.Resolver{},
		cache:   resolverCache,
		cacheMu: &sync.Mutex{},
	}
}

// LookupHost looks up the specified hostname.
func (r *Resolver) LookupHost(host string) (ips []net.IP, err error) {
	// TODO(ameshkov): refactor, move to smaller methods.
	var ok bool
	if ips, ok = r.lookupCache(host); ok {
		return ips, nil
	}

	ips, err = r.res.LookupIP(context.Background(), "ip4", host)

	if err == nil && len(ips) > 0 {
		r.putToCache(host, ips)
	}

	return ips, err
}

func (r *Resolver) lookupCache(host string) (ips []net.IP, ok bool) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	ips, ok = r.cache[host]

	return ips, ok
}

func (r *Resolver) putToCache(host string, ips []net.IP) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	r.cache[host] = ips
}
