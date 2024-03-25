package relay

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// peekServerName peeks on the first bytes from the reader and tries to parse
// the remote server name.  Depending on whether this is a TLS or a plain HTTP
// connection it will use different ways of parsing.
func peekServerName(
	reader io.Reader,
	plainHTTP bool,
) (serverName string, newReader io.Reader, err error) {
	if plainHTTP {
		serverName, newReader, err = peekHTTPHost(reader)

		if err != nil {
			return "", nil, err
		}
	} else {
		var clientHello *tls.ClientHelloInfo
		clientHello, newReader, err = peekClientHello(reader)

		if err != nil {
			return "", nil, err
		}

		serverName = clientHello.ServerName
	}

	return serverName, newReader, nil
}

// peekHTTPHost peeks on the first bytes from the reader and tries to parse the
// HTTP Host header.  Once it's done, it returns the hostname and a new reader
// that contains unmodified data.
func peekHTTPHost(reader io.Reader) (host string, newReader io.Reader, err error) {
	peekedBytes := new(bytes.Buffer)
	teeReader := bufio.NewReader(io.TeeReader(reader, peekedBytes))

	r, err := http.ReadRequest(teeReader)
	if err != nil {
		return "", nil, fmt.Errorf("sniproxy: failed to read http request: %w", err)
	}

	return r.Host, io.MultiReader(peekedBytes, reader), nil
}

// peekClientHello peeks on the first bytes from the reader and tries to parse
// the TLS ClientHello.  Once it's done, it returns the client hello information
// and a new reader that contains unmodified data.
func peekClientHello(
	reader io.Reader,
) (hello *tls.ClientHelloInfo, newReader io.Reader, err error) {
	peekedBytes := new(bytes.Buffer)
	hello, err = readClientHello(io.TeeReader(reader, peekedBytes))
	if err != nil {
		return nil, nil, err
	}

	return hello, io.MultiReader(peekedBytes, reader), nil
}

// readClientHello reads client hello information from the specified reader.
func readClientHello(reader io.Reader) (hello *tls.ClientHelloInfo, err error) {
	err = tls.Server(readOnlyConn{reader: reader}, &tls.Config{
		GetConfigForClient: func(argHello *tls.ClientHelloInfo) (*tls.Config, error) {
			hello = new(tls.ClientHelloInfo)
			*hello = *argHello
			return nil, nil
		},
	}).Handshake()

	if hello == nil {
		return nil, err
	}

	return hello, nil
}

// readOnlyConn implements net.Conn but overrides all it's methods so that
// only reading could work.  The purpose is to make sure that the Handshake
// method of [tls.Server] does not write any data to the underlying connection.
type readOnlyConn struct {
	reader io.Reader
}

// type check
var _ net.Conn = (*readOnlyConn)(nil)

// Read implements the net.Conn interface for *readOnlyConn.
func (conn readOnlyConn) Read(p []byte) (int, error) { return conn.reader.Read(p) }

// Write implements the net.Conn interface for *readOnlyConn.
func (conn readOnlyConn) Write(_ []byte) (int, error) { return 0, io.ErrClosedPipe }

// Close implements the net.Conn interface for *readOnlyConn.
func (conn readOnlyConn) Close() error { return nil }

// LocalAddr implements the net.Conn interface for *readOnlyConn.
func (conn readOnlyConn) LocalAddr() net.Addr { return nil }

// RemoteAddr implements the net.Conn interface for *readOnlyConn.
func (conn readOnlyConn) RemoteAddr() net.Addr { return nil }

// SetDeadline implements the net.Conn interface for *readOnlyConn.
func (conn readOnlyConn) SetDeadline(_ time.Time) error { return nil }

// SetReadDeadline implements the net.Conn interface for *readOnlyConn.
func (conn readOnlyConn) SetReadDeadline(_ time.Time) error { return nil }

// SetWriteDeadline implements the net.Conn interface for *readOnlyConn.
func (conn readOnlyConn) SetWriteDeadline(_ time.Time) error { return nil }
