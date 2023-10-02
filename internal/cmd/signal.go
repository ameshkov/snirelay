package cmd

import (
	"os"
	"os/signal"

	"bit.int.agrd.dev/relay/internal/relay"

	"github.com/AdguardTeam/golibs/log"
	"golang.org/x/sys/unix"
)

// signalHandler processes incoming signals and shuts services down.
type signalHandler struct {
	signal chan os.Signal

	// services are the services that are shut down before application
	// exiting.
	services []*relay.Server
}

// Exit status constants.
const (
	statusSuccess = 0
	statusError   = 1
)

// handle processes OS signals.  status is [statusSuccess] on success and
// [statusError] on error.
func (h *signalHandler) handle() (status int) {
	defer log.OnPanic("signalHandler.handle")

	for sig := range h.signal {
		log.Info("sighdlr: received signal %q", sig)

		switch sig {
		case
			unix.SIGINT,
			unix.SIGQUIT,
			unix.SIGTERM:
			return h.shutdown()
		}
	}

	// Shouldn't happen, since h.signal is currently never closed.
	return statusError
}

// shutdown gracefully shuts down all services.  status is [statusSuccess] on
// success and [statusError] on error.
func (h *signalHandler) shutdown() (status int) {
	log.Info("sighdlr: shutting down services")
	for i, service := range h.services {
		err := service.Close()
		if err != nil {
			log.Error("sighdlr: shutting down service at index %d: %s", i, err)
			status = statusError
		}
	}

	log.Info("sighdlr: shutting down")

	return status
}

// newSignalHandler returns a new signalHandler that shuts down svcs.
func newSignalHandler(svcs ...*relay.Server) (h signalHandler) {
	h = signalHandler{
		signal:   make(chan os.Signal, 1),
		services: svcs,
	}

	signal.Notify(h.signal, unix.SIGINT, unix.SIGQUIT, unix.SIGTERM)

	return h
}
