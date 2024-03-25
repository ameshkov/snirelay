// Package relay implements all the relay logic.
package relay

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/AdguardTeam/golibs/netutil"

	"golang.org/x/net/proxy"

	"github.com/AdguardTeam/golibs/errors"
	"github.com/AdguardTeam/golibs/log"
)

const (
	// readTimeout is the default timeout for a connection read deadline.
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
	started bool
	wg      *sync.WaitGroup

	dialer        proxy.Dialer
	resolverCache map[string][]net.IP

	listenAddrPlain *net.TCPAddr
	listenerPlain   net.Listener

	listenAddrTLS *net.TCPAddr
	listenerTLS   net.Listener

	// mu protects started and listeners.
	mu *sync.Mutex
}

// type check.
var _ io.Closer = (*Server)(nil)

// NewServer creates a new instance of *Server.
func NewServer(
	listenAddr string,
	listenPort int,
	listenPortTLS int,
	proxyURL *url.URL,
	resolverCache map[string][]net.IP,
) (s *Server, err error) {
	s = &Server{
		wg:            &sync.WaitGroup{},
		mu:            &sync.Mutex{},
		resolverCache: resolverCache,
	}

	s.dialer = proxy.Direct
	if proxyURL != nil {
		s.dialer, err = proxy.FromURL(proxyURL, s.dialer)

		if err != nil {
			return nil, fmt.Errorf("invalid proxy: %w", err)
		}
	}

	listenIP := net.ParseIP(listenAddr)
	if listenIP == nil {
		return nil, fmt.Errorf("invalid listen IP: %s", listenAddr)
	}

	s.listenAddrPlain = &net.TCPAddr{
		IP:   listenIP,
		Port: listenPort,
	}

	s.listenAddrTLS = &net.TCPAddr{
		IP:   listenIP,
		Port: listenPortTLS,
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

	return s.listenerTLS.Addr()
}

// AddrPlain returns the address where the server listens for plain traffic.
func (s *Server) AddrPlain() (addr net.Addr) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	return s.listenerPlain.Addr()
}

// Start starts the server.
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

	s.listenerTLS, err = net.ListenTCP("tcp", s.listenAddrTLS)
	if err != nil {
		return fmt.Errorf("failed to serve TLS: %w", err)
	}

	s.wg.Add(2)

	go func() {
		defer s.wg.Done()

		// TODO(ameshkov): Handle error here.
		_ = s.acceptLoop(s.listenerPlain, true)
	}()

	go func() {
		defer s.wg.Done()

		// TODO(ameshkov): Handle error here.
		_ = s.acceptLoop(s.listenerTLS, false)
	}()

	s.started = true

	log.Info("relay: started")

	return nil
}

// acceptLoop runs the infinite accept loop for TLS or HTTP traffic.
func (s *Server) acceptLoop(l net.Listener, plainHTTP bool) (err error) {
	for {
		var conn net.Conn
		conn, err = l.Accept()

		if errors.Is(err, net.ErrClosed) {
			log.Info("relay: exiting listener loop as it has been closed")

			return err
		}

		if err == nil {
			go func() {
				hErr := s.handleConn(conn, plainHTTP)
				if hErr != nil {
					log.Debug("failed to handle conn: %v", hErr)
				}
			}()
		}
	}
}

// getRemoteAddr checks if there is already a mapping for the specified server
// name and returns the IP address if so.  Returns the same server name when
// there's no mapping.  Appends the correct port depending on what protocol is
// in use.
func (s *Server) getRemoteAddr(serverName string, plainHTTP bool) (remoteAddr string) {
	remoteAddr = serverName

	if v, ok := s.resolverCache[serverName]; ok {
		remoteAddr = v[0].String()
	}

	if plainHTTP {
		return netutil.JoinHostPort(remoteAddr, remotePortPlain)
	}

	return netutil.JoinHostPort(remoteAddr, remotePortTLS)
}

// handleConn handles the network connection, peeks SNI and tunnels traffic.
func (s *Server) handleConn(conn net.Conn, plainHTTP bool) (err error) {
	defer log.OnCloserError(conn, log.DEBUG)

	log.Debug("relay: accepting new connection from %s", conn.RemoteAddr())

	if err = conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		return fmt.Errorf("relay: failed to set read deadline: %w", err)
	}

	serverName, clientReader, err := peekServerName(conn, plainHTTP)
	if err != nil {
		return fmt.Errorf("relay: failed to peek server name: %w", err)
	}

	log.Debug("relay: server name is %s", serverName)

	if err = conn.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("relay: failed to remove read deadline: %w", err)
	}

	remoteAddr := s.getRemoteAddr(serverName, plainHTTP)
	log.Debug("relay: connecting to %s", remoteAddr)

	var remoteConn net.Conn
	remoteConn, err = s.dialer.Dial("tcp", remoteAddr)
	if err != nil {
		return fmt.Errorf("relay: failed to connect to %s: %w", remoteAddr, err)
	}

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

		bytesSent = s.tunnel(remoteConn, clientReader)
	}()

	wg.Wait()

	elapsed := time.Now().Sub(startTime)

	log.Debug(
		"relay: finished tunneling to %s. received %d, sent %d, elapsed: %v",
		remoteAddr,
		bytesReceived,
		bytesSent,
		elapsed,
	)

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
