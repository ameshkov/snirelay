// Package relay implements all the SNI relay logic.
package relay

import (
	"fmt"
	"io"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"github.com/IGLOU-EU/go-wildcard"

	"github.com/AdguardTeam/golibs/errors"
	"github.com/AdguardTeam/golibs/log"
	"github.com/AdguardTeam/golibs/netutil"
	"github.com/ameshkov/snirelay/internal/metrics"
	"github.com/getsentry/sentry-go"
	"golang.org/x/net/proxy"
)

const (
	// readTimeout is the default timeout for a connection read deadline.
	//
	// TODO(ameshkov): Consider making configurable.
	readTimeout = 60 * time.Second

	// remotePortPlain is the port the proxy will be connecting for plain
	// HTTP connections.
	remotePortPlain = 80

	// remotePortTLS is the port the proxy will be connecting to for TLS
	// connections.
	remotePortTLS = 443
)

// Server implements all the relay logic, listens for incoming connections and
// redirects them to the proper server.
type Server struct {
	redirectDomains []string

	dialer          proxy.Dialer
	listenAddrPlain *net.TCPAddr
	listenerPlain   net.Listener
	plainAddr       net.Addr
	listenAddrTLS   *net.TCPAddr
	listenerTLS     net.Listener
	tlsAddr         net.Addr

	// mu protects started and listeners.
	mu *sync.Mutex

	// wg keeps track of the active connections.
	wg *sync.WaitGroup

	started bool
}

// type check.
var _ io.Closer = (*Server)(nil)

// NewServer creates a new instance of *Server.
func NewServer(cfg *Config) (s *Server, err error) {
	s = &Server{
		redirectDomains: cfg.RedirectDomains,
		wg:              &sync.WaitGroup{},
		mu:              &sync.Mutex{},
	}

	if cfg.ProxyURL != nil {
		s.dialer, err = proxy.FromURL(cfg.ProxyURL, s.dialer)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy: %w", err)
		}
	}

	s.listenAddrPlain = &net.TCPAddr{
		IP:   cfg.ListenAddr.AsSlice(),
		Port: int(cfg.ListenPort),
	}

	s.listenAddrTLS = &net.TCPAddr{
		IP:   cfg.ListenAddr.AsSlice(),
		Port: int(cfg.ListenPortTLS),
	}

	return s, nil
}

// AddrTLS returns the address where the server listens for TLS traffic.
func (s *Server) AddrTLS() (addr net.Addr) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	return s.tlsAddr
}

// AddrPlain returns the address where the server listens for plain traffic.
func (s *Server) AddrPlain() (addr net.Addr) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	return s.plainAddr
}

// Start starts the relay server.
func (s *Server) Start() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Info("relay: starting")

	if s.started {
		return fmt.Errorf("server is already started")
	}

	s.listenerPlain, err = net.ListenTCP("tcp", s.listenAddrPlain)
	if err != nil {
		return fmt.Errorf("failed to serve plain HTTP: %w", err)
	}
	s.plainAddr = s.listenerPlain.Addr()

	s.listenerTLS, err = net.ListenTCP("tcp", s.listenAddrTLS)
	if err != nil {
		return fmt.Errorf("failed to serve TLS: %w", err)
	}
	s.tlsAddr = s.listenerTLS.Addr()

	s.wg.Add(2)

	go s.acceptLoop(s.listenerPlain, true)
	go s.acceptLoop(s.listenerTLS, false)

	s.started = true

	log.Info("relay: listening for plain HTTP on %s", s.listenerPlain.Addr())
	log.Info("relay: listening for TLS on %s", s.listenerTLS.Addr())

	return nil
}

// acceptLoop runs the infinite accept loop for TLS or HTTP traffic.
func (s *Server) acceptLoop(l net.Listener, plainHTTP bool) {
	defer s.wg.Done()

	for {
		conn, err := l.Accept()

		if errors.Is(err, net.ErrClosed) {
			log.Info("relay: exiting listener loop as it has been closed")

			return
		}

		if err == nil {
			go s.handleConn(conn, plainHTTP)
		} else {
			// TODO(ameshkov): There is a risk of a busy loop, consider fixing.
			log.Debug("relay: error accepting conn: %v", err)
		}

	}
}

// handlePanicAndRecover is a helper function that recovers from panics and
// reports them to Sentry.
func handlePanicAndRecover() {
	if v := recover(); v != nil {
		log.Error(
			"panic encountered in the relay server, recovered: %s\n%s",
			v,
			string(debug.Stack()),
		)

		// TODO(ameshkov): refactor and add scope and tags to sentry event.
		sentry.CaptureMessage(fmt.Sprintf("panic encountered in the relay server: %v", v))
	}
}

// handleConn handles incoming connection.
func (s *Server) handleConn(conn net.Conn, plainHTTP bool) {
	defer handlePanicAndRecover()

	hErr := s.handleRelayConn(conn, plainHTTP)
	if hErr != nil {
		log.Error("relay: failed to handle conn: %v", hErr)

		sentry.CaptureException(hErr)
	}
}

// remoteAddrForServerName creates a remote address string depending on the
// protocol.
func remoteAddrForServerName(serverName string, plainHTTP bool) (remoteAddr string) {
	if plainHTTP {
		return netutil.JoinHostPort(serverName, remotePortPlain)
	}

	return netutil.JoinHostPort(serverName, remotePortTLS)
}

// handleRelayConn handles the network connection, peeks SNI and tunnels
// traffic.
func (s *Server) handleRelayConn(conn net.Conn, plainHTTP bool) (err error) {
	defer log.OnCloserError(conn, log.DEBUG)

	log.Debug("relay: accepting new connection from %s", conn.RemoteAddr())

	if err = conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}

	serverName, connReader, err := peekServerName(conn, plainHTTP)
	if err != nil {
		return fmt.Errorf("failed to peek server name: %w", err)
	}

	log.Debug("relay: peeked server name is %q", serverName)

	if err = conn.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("failed to remove read deadline: %w", err)
	}

	if !s.canAccept(serverName) {
		log.Debug("relay: relaying %s is not allowed", serverName)

		return nil
	}

	if serverName == s.plainAddr.String() ||
		serverName == s.tlsAddr.String() {
		log.Debug("relay: direct connection to the relay IP, closing it")

		return nil
	}

	remoteAddr := remoteAddrForServerName(serverName, plainHTTP)
	log.Debug("relay: connecting to %s", remoteAddr)

	return s.handleConnToRemoteServer(conn, connReader, remoteAddr)
}

// connect opens a connection to the specified remote address.  By default, it
// tries to bind to the same network interface it received the source connection
// from if this is a public IP.  The reason for that is that the server may
// have multiple IP addresses, and it may be required to control which of them
// is used.
func (s *Server) connect(localAddr net.Addr, remoteAddr string) (conn net.Conn, err error) {
	if s.dialer != nil {
		// If a proxy dialer is set it does not matter what network interface
		// is used.
		return s.dialer.Dial("tcp", remoteAddr)
	}

	// snirelay only works with TCP so there is no need to check for other
	// types of addresses.
	localIP := localAddr.(*net.TCPAddr).IP

	var bindAddr *net.TCPAddr
	if !localIP.IsPrivate() &&
		!localIP.IsLoopback() &&
		!localIP.IsUnspecified() {
		bindAddr = &net.TCPAddr{
			IP: localIP,
		}
	}

	dialer := &net.Dialer{
		LocalAddr: bindAddr,
	}

	return dialer.Dial("tcp", remoteAddr)
}

// handleConnToRemoteServer connects to the remote address remoteAddr and then
// tunnels traffic from the client connection conn.
func (s *Server) handleConnToRemoteServer(
	conn net.Conn,
	connReader io.Reader,
	remoteAddr string,
) (err error) {
	var remoteConn net.Conn
	remoteConn, err = s.connect(conn.LocalAddr(), remoteAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", remoteAddr, err)
	}

	metrics.ConnectionsTotal.WithLabelValues(remoteAddr).Inc()
	defer func() {
		metrics.ConnectionsTotal.WithLabelValues(remoteAddr).Dec()

		// Make sure the remote connection is closed.
		log.OnCloserError(remoteConn, log.DEBUG)
	}()

	clientAddr := netutil.NetAddrToAddrPort(conn.RemoteAddr())
	metrics.RelayUsersCountUpdate(clientAddr.Addr())

	startTime := time.Now()

	log.Debug("relay: start tunneling to %s", remoteAddr)

	var wg sync.WaitGroup
	wg.Add(2)

	log.Debug("relay: start tunneling %s<->%s", remoteAddr, conn.RemoteAddr())

	var bytesReceived, bytesSent int64

	go func() {
		defer wg.Done()

		bytesReceived = s.tunnel(conn, remoteConn)
	}()

	go func() {
		defer wg.Done()

		bytesSent = s.tunnel(remoteConn, connReader)
	}()

	wg.Wait()

	elapsed := time.Since(startTime)

	log.Debug(
		"relay: finished tunneling to %s. received %d, sent %d, elapsed: %v",
		remoteAddr,
		bytesReceived,
		bytesSent,
		elapsed,
	)

	metrics.BytesSentTotal.WithLabelValues(remoteAddr).Add(float64(bytesSent))
	metrics.BytesReceivedTotal.WithLabelValues(remoteAddr).Add(float64(bytesReceived))

	return nil
}

// closeWriter is a helper interface which only purpose is to check if the
// object has CloseWrite function or not and call it if it exists.
type closeWriter interface {
	CloseWrite() error
}

// copy copies data from src to dst and signals that the work is done via the
// wg wait group.
func (s *Server) tunnel(dst net.Conn, src io.Reader) (written int64) {
	defer func() {
		// In the case of *tcp.Conn and *tls.Conn we should call CloseWriter, so
		// we're using closeWriter interface to check for that function
		// presence.
		switch c := dst.(type) {
		case closeWriter:
			_ = c.CloseWrite()
		default:
			_ = c.Close()
		}
	}()

	written, err := io.Copy(dst, src)
	if err != nil {
		log.Debug("relay: finished copying due to %v", err)
	}

	return written
}

// Close implements the io.Closer interface for *Server.
func (s *Server) Close() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Info("relay: closing")

	if !s.started {
		return nil
	}

	plainErr := s.listenerPlain.Close()
	tlsErr := s.listenerTLS.Close()

	log.Info("relay: waiting until connections stop processing")

	s.wg.Wait()

	log.Info("relay: closed")

	return errors.Join(plainErr, tlsErr)
}

// canAccept checks if relay can accept connection to this hostname.
func (s *Server) canAccept(hostname string) (ok bool) {
	for _, pattern := range s.redirectDomains {
		if wildcard.MatchSimple(pattern, hostname) {
			return true
		}
	}

	return false
}
