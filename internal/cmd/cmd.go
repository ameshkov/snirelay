// Package cmd is responsible for the program's command-line interface.
package cmd

import (
	"fmt"
	"os"

	"github.com/AdguardTeam/golibs/errors"
	"github.com/AdguardTeam/golibs/log"
	"github.com/ameshkov/snirelay/internal/config"
	"github.com/ameshkov/snirelay/internal/dnssrv"
	"github.com/ameshkov/snirelay/internal/relay"
	"github.com/ameshkov/snirelay/internal/version"
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

	check("parse args", err)

	if o.Verbose {
		log.SetLevel(log.DEBUG)
	}

	cfg, err := config.Load(o.ConfigPath)
	check("load config file", err)

	relayCfg, err := cfg.ToRelayConfig()
	check("parse relay config", err)

	dnsCfg, err := cfg.ToDNSConfig()
	check("parse dns config", err)

	relaySrv, err := relay.NewServer(relayCfg)
	check("init relay server", err)

	err = relaySrv.Start()
	check("start relay server", err)

	if dnsCfg != nil {
		dnsSrv, err := dnssrv.New(dnsCfg)
		check("init dns server", err)

		err = dnsSrv.Start()
		check("start dns server", err)
	}

	sigHandler := newSignalHandler(relaySrv)
	os.Exit(sigHandler.handle())
}

// check panics if err is not nil.
func check(operationName string, err error) {
	if err != nil {
		log.Error("failed to %s: %v", operationName, err)

		os.Exit(1)
	}
}
