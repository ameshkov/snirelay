// Package relay implements all the relay logic.
package relay

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/AdguardTeam/golibs/errors"
	"github.com/AdguardTeam/golibs/log"
	"src.agwa.name/tlshacks"
)

const (
	// ErrNotTLS is returned when the relay cannot peek the server name.
	ErrNotTLS = errors.Error("not a TLS handshake")

	// ErrNoServerName is returned when handshake does not contain a server name
	// extension.
	ErrNoServerName = errors.Error("no server name extension")

	// readTimeout is the default timeout for a connection read deadline.
	readTimeout = 10 * time.Second

	// connectionTimeout is a timeout for connecting to a remote host.
	connectionTimeout = 10 * time.Second

	// remotePortTLS is the port the proxy will be connecting to for TLS
	// connection.
	remotePortTLS = 443
)

// Server implements all the relay logic, listens for incoming connections and
// redirects them to the proper server.
type Server struct {
	listenAddr *net.TCPAddr
	dialer     *net.Dialer
	resolver   *Resolver

	listener net.Listener
}

// type check.
var _ io.Closer = (*Server)(nil)

// NewServer creates a new instance of *Server.
func NewServer(
	listenAddr string,
	listenPort uint16,
	resolverCache map[string][]net.IP,
) (s *Server, err error) {
	s = &Server{}
	s.resolver = NewResolver(resolverCache)
	s.dialer = &net.Dialer{Timeout: connectionTimeout}
	s.listenAddr = &net.TCPAddr{
		IP:   net.ParseIP(listenAddr),
		Port: int(listenPort),
	}

	return s, nil
}

// Serve starts the listener and accepts incoming connections. This method is
// synchronous, it returns an error when the server is closed.
func (s *Server) Serve() (err error) {
	s.listener, err = net.ListenTCP("tcp", s.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to server: %w", err)
	}

	return s.acceptLoop(s.listener)
}

// acceptLoop runs the infinite accept loop
func (s *Server) acceptLoop(l net.Listener) (err error) {
	for {
		var conn net.Conn
		conn, err = l.Accept()

		if errors.Is(err, net.ErrClosed) {
			log.Info("relay: exiting listener loop as it has been closed")

			return err
		}

		if err == nil {
			go func() {
				hErr := s.handleConn(conn)
				if hErr != nil {
					log.Debug("failed to handle conn: %v", hErr)
				}
			}()
		}
	}
}

// handleConn handles the network connection, peeks SNI and tunnels traffic.
func (s *Server) handleConn(conn net.Conn) (err error) {
	defer log.OnCloserError(conn, log.DEBUG)

	if err = conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		return fmt.Errorf("relay: failed to set read deadline: %w", err)
	}

	serverName, clientReader, err := peekServerName(conn)
	if err != nil {
		return fmt.Errorf("relay: failed to peek server name: %w", err)
	}

	if err = conn.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("relay: failed to remove read deadline: %w", err)
	}

	var ips []net.IP
	if ips, err = s.resolver.LookupHost(serverName); err != nil {
		return fmt.Errorf("relay: failed to resolve %s: %w", serverName, err)
	}

	remoteAddr := &net.TCPAddr{
		IP:   ips[0],
		Port: remotePortTLS,
	}

	var remoteConn net.Conn
	remoteConn, err = s.dialer.Dial("tcp", remoteAddr.String())
	if err != nil {
		return fmt.Errorf("relay: failed to connect to %s: %w", remoteAddr, err)
	}

	startTime := time.Now()

	log.Debug("relay: start tunneling to %s", remoteAddr)

	var wg sync.WaitGroup
	wg.Add(2)

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

// peekServerName peeks on the first bytes from the reader and tries to parse
// the remote server name.
func peekServerName(reader io.Reader) (serverName string, newReader io.Reader, err error) {
	peekedBytes := new(bytes.Buffer)
	r := io.TeeReader(reader, peekedBytes)

	// TODO(ameshkov): use sync.Pool here.
	buf := make([]byte, 2048)
	var bytesRead int
	bytesRead, err = io.ReadAtLeast(r, buf, 5)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read ClientHello: %w", err)
	}

	if buf[0] != 0x16 {
		return "", nil, ErrNotTLS
	}

	// Length of the handshake message.
	length := int(binary.BigEndian.Uint16(buf[3:5]))
	toRead := length + 5 - bytesRead
	if toRead > 0 {
		_, err = io.ReadAtLeast(r, buf[bytesRead:], toRead)
		if err != nil {
			return "", nil, fmt.Errorf("failed to read the handshake bytes: %w", err)
		}
	}

	clientHello := tlshacks.UnmarshalClientHello(buf[5 : length+5])
	if clientHello == nil {
		return "", nil, ErrNotTLS
	}

	if clientHello.Info.ServerName == nil {
		return "", nil, ErrNoServerName
	}

	return *clientHello.Info.ServerName, io.MultiReader(peekedBytes, reader), err
}

// Close implements the io.Closer interface for *Server.
func (s *Server) Close() (err error) {
	// TODO(ameshkov): wait until all connections are processed.

	return s.listener.Close()
}
