// Package cmd is responsible for the program's command-line interface.
package cmd

import (
	"encoding/csv"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"bit.int.agrd.dev/relay/internal/relay"
	"bit.int.agrd.dev/relay/internal/version"
	"github.com/AdguardTeam/golibs/errors"
	"github.com/AdguardTeam/golibs/log"
	goFlags "github.com/jessevdk/go-flags"
)

// Main is the entry point of the program.
func Main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("snirelay version: %s\n", version.Version())

		os.Exit(0)
	}

	o, err := parseOptions()
	var flagErr *goFlags.Error
	if errors.As(err, &flagErr) && flagErr.Type == goFlags.ErrHelp {
		// This is a special case when we exit process here as we received
		// --help.
		os.Exit(0)
	}

	if err != nil {
		log.Error("cmd: failed to parse args: %v", err)

		os.Exit(1)
	}

	if o.Verbose {
		log.SetLevel(log.DEBUG)
	}

	log.Info("cmd: snirelay configuration:\n%s", o)

	resolverCache := map[string][]net.IP{}

	if o.SNIMappingsPath != "" {
		f, err := os.Open(o.SNIMappingsPath)
		check(err)

		r := csv.NewReader(f)
		records, err := r.ReadAll()
		check(err)

		for _, rec := range records {
			domain := rec[0]
			ip := net.ParseIP(rec[1])
			resolverCache[domain] = []net.IP{ip}
		}
	}

	ports := strings.Split(o.Ports, ":")
	if len(ports) != 2 {
		log.Error("cmd: invalid ports: %s", o.Ports)

		os.Exit(1)
	}

	plainPort, err := strconv.Atoi(ports[0])
	if err != nil {
		log.Error("cmd: failed to parse plain port: %v", err)

		os.Exit(1)
	}

	tlsPort, err := strconv.Atoi(ports[1])
	if err != nil {
		log.Error("cmd: failed to parse TLS port: %v", err)

		os.Exit(1)
	}

	log.Info("cmd: resolver cache size: %d", len(resolverCache))
	log.Info("cmd: listening for HTTP requests on %s:%d", o.ListenAddr, plainPort)
	log.Info("cmd: listening for TLS connections on %s:%d", o.ListenAddr, tlsPort)

	s, err := relay.NewServer(o.ListenAddr, plainPort, tlsPort, resolverCache)
	check(err)

	err = s.Start()
	check(err)

	sigHandler := newSignalHandler(s)
	os.Exit(sigHandler.handle())
}

// check panics if err is not nil.
func check(err error) {
	if err != nil {
		panic(err)
	}
}
