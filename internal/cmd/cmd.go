// Package cmd is responsible for the program's command-line interface.
package cmd

import (
	"encoding/csv"
	"net"
	"os"

	"bit.int.agrd.dev/relay/internal/relay"
	"github.com/AdguardTeam/golibs/log"
)

// Main is the entry point of the program.
func Main() {
	envs, err := readEnvs()
	check(err)

	if envs.LogVerbose {
		log.SetLevel(log.DEBUG)
	}

	f, err := os.Open(envs.SNIMappingCSVPath)
	check(err)

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	check(err)

	resolverCache := map[string][]net.IP{}
	for _, rec := range records {
		domain := rec[0]
		ip := net.ParseIP(rec[1])
		resolverCache[domain] = []net.IP{ip}
	}

	log.Info("cmd: resolver cache size: %d", len(resolverCache))
	log.Info("cmd: starting relay on %s:%d", envs.ListenAddr, envs.ListenPort)

	s, err := relay.NewServer(envs.ListenAddr, envs.ListenPort, resolverCache)
	check(err)

	go func() {
		err = s.Serve()
		log.Info("cmd: finished serving due to %v", err)
	}()

	sigHandler := newSignalHandler(s)
	os.Exit(sigHandler.handle())
}

// check panics if err is not nil.
func check(err error) {
	if err != nil {
		panic(err)
	}
}
