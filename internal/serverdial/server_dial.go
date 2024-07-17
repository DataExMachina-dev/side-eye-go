package serverdial

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"
)

// Listener implements net.Listener and dials connections to a remote address.
// Listener is a surprising guy -- it doesn't actually "listen" for any sort of
// incoming connections. Instead, it dials a single connection to a remote
// address. When that connection is established, it sends a handshake packet
// (informing the remote server that it should use it as a gRPC client conn) and
// then it returns the connection from the Accept(). When the connection drops,
// it dials a new one (so, there's ever at most one connection active).
type Listener struct {
	addr     serverDialAddr
	dialChan <-chan net.Conn

	dialingCtx context.Context
	// cancelDialing is used by Close() to signal the run() goroutine to
	// terminate.
	cancelDialing context.CancelFunc
	// The done channel is used by Close() to synchronize with the run()
	// goroutine.
	done <-chan struct{}

	mu struct {
		sync.Mutex
		status ConnectionStatus
	}
}

var _ net.Listener = (*Listener)(nil)

type ConnectionStatus int

// NOTE: Keep enum in sync with sideeye.ConnectionStatus.
const (
	UnknownStatus ConnectionStatus = iota
	Connected
	Disconnected
	Connecting
)

// NewListener creates a Listener that dials the given address. Note that the
// address should be a valid URL with either http or https scheme and no path or
// query.
//
// A goroutine is started which dials the target asynchronously. When a
// connection drops, a new one is dialed.
//
// errLogger can be nil.
func NewListener(
	addr string,
	errLogger func(error),
) (*Listener, error) {
	dialChan := make(chan net.Conn)
	done := make(chan struct{})
	d, sdAddr, err := newDialer(addr)
	if err != nil {
		return nil, err
	}
	if errLogger == nil {
		errLogger = func(error) {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	sd := &Listener{
		addr:          sdAddr,
		cancelDialing: cancel,
		dialingCtx:    ctx,
		done:          done,
		dialChan:      dialChan,
	}
	go func() {
		defer close(done)
		defer sd.setConnectionStatus(Disconnected)
		sd.run(ctx, sdAddr, d, dialChan, errLogger)
	}()
	return sd, err
}

// ConnectionStatus returns the connection's current state.
func (l *Listener) ConnectionStatus() ConnectionStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.mu.status
}

func (l *Listener) setConnectionStatus(status ConnectionStatus) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.mu.status = status
}

func (l *Listener) run(
	ctx context.Context,
	addr serverDialAddr,
	d dialer,
	dialChan chan<- net.Conn,
	errLogger func(error),
) {
	l.setConnectionStatus(Connecting)
	// TODO: Exponential backoff or something like that.
	const dialInterval = 2 * time.Second
	var lastDial time.Time
	for {
		if since := time.Since(lastDial); since < dialInterval {
			select {
			case <-ctx.Done():
				return
			case <-time.After(dialInterval - since):
			}
		}
		lastDial = time.Now()
		conn, err := d.DialContext(ctx, "tcp", addr.addr)
		if err != nil {
			errLogger(fmt.Errorf("failed to dial %s: %w", addr.addr, err))
			continue
		}
		l.setConnectionStatus(Connected)
		onClose := make(chan struct{})
		dialChan <- &wrappedConn{
			c:       conn,
			onClose: onClose,
		}
		select {
		case <-ctx.Done():
			return
		case <-onClose:
		}
		l.setConnectionStatus(Connecting)
	}
}

// wrappedConn wraps a net.Conn
type wrappedConn struct {
	// The underlying connection.
	c net.Conn

	closeOnce sync.Once
	// onClose is closed when the connection's Close() method is called.
	onClose  chan<- struct{}
	closeErr error
}

var _ net.Conn = (*wrappedConn)(nil)

// LocalAddr implements net.Conn.
func (w *wrappedConn) LocalAddr() net.Addr {
	return w.c.LocalAddr()
}

// Read implements net.Conn.
func (w *wrappedConn) Read(b []byte) (n int, err error) {
	return w.c.Read(b)
}

// RemoteAddr implements net.Conn.
func (w *wrappedConn) RemoteAddr() net.Addr {
	return w.c.RemoteAddr()
}

// SetDeadline implements net.Conn.
func (w *wrappedConn) SetDeadline(t time.Time) error {
	return w.c.SetDeadline(t)
}

// SetReadDeadline implements net.Conn.
func (w *wrappedConn) SetReadDeadline(t time.Time) error {
	return w.c.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn.
func (w *wrappedConn) SetWriteDeadline(t time.Time) error {
	return w.c.SetWriteDeadline(t)
}

// Write implements net.Conn.
func (w *wrappedConn) Write(b []byte) (n int, err error) {
	return w.c.Write(b)
}

// Close implements net.Conn.
//
// We're relying on gPRC to call Close() if the wrapped connection drops.
func (w *wrappedConn) Close() error {
	w.closeOnce.Do(func() {
		w.closeErr = w.c.Close()
		close(w.onClose)
	})
	return w.closeErr
}

// Accept implements net.Listener.
func (l *Listener) Accept() (net.Conn, error) {
	select {
	case <-l.dialingCtx.Done():
		return nil, l.dialingCtx.Err()
	case conn := <-l.dialChan:
		return conn, nil
	}
}

// Addr implements net.Listener.
func (l *Listener) Addr() net.Addr {
	return &l.addr
}

type serverDialAddr struct {
	scheme string
	addr   string
}

// dialer abstracts over net.Dialer vs https.Dialer.
type dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// newDialer returns a dialer that:
// a) abstracts over http versus https, and
// b) sends the magic header on every dialed connection that identifies the
// connection as being a "server-dialed" one -- i.e. the party dialing the
// connection will serve a gRPC server on the connection, so the target of the
// connection actually acts as the client from gRPC's perspective.
func newDialer(addr string) (dialer, serverDialAddr, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, serverDialAddr{}, fmt.Errorf("failed to parse url: %w", err)
	}
	if u.Path != "" {
		return nil, serverDialAddr{}, fmt.Errorf("unsupported path: %s", u.Path)
	}
	if u.RawQuery != "" {
		return nil, serverDialAddr{}, fmt.Errorf("unsupported query: %s", u.RawQuery)
	}
	var d dialer
	switch u.Scheme {
	case "http":
		d = &net.Dialer{}
	case "https":
		d = &tls.Dialer{}
	default:
		return nil, serverDialAddr{}, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	// Whenever we
	dialer := &headerDialer{d: d}
	return dialer, serverDialAddr{
		scheme: u.Scheme,
		addr:   u.Host,
	}, nil
}

// Network implements net.Addr.
func (s *serverDialAddr) Network() string {
	return "serverdial"
}

// String implements net.Addr.
func (s *serverDialAddr) String() string {
	return s.addr
}

// Close implements net.Listener.
func (l *Listener) Close() error {
	l.cancelDialing()
	<-l.done
	return nil
}
