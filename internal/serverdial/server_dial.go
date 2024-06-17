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

// Listener implements net.Listener and dials connections to an
// outbound address.
type Listener struct {
	addr     serverDialAddr
	dialChan <-chan net.Conn

	dialingCtx    context.Context
	cancelDialing context.CancelFunc
	done          <-chan struct{}
}

// NewServerDialListener creates a ServerDialListener that dials the given
// address. Note that the address should be a valid URL with either http or
// https scheme and no path or query.
func NewListener(
	addr string,
	onDialError func(error),
) (net.Listener, error) {
	dialChan := make(chan net.Conn)
	done := make(chan struct{})
	d, sdAddr, err := newDialer(addr)
	if err != nil {
		return nil, err
	}
	if onDialError == nil {
		onDialError = func(error) {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	sd := &Listener{
		addr:          sdAddr,
		cancelDialing: cancel,
		dialingCtx:    ctx,
		done:          done,
		dialChan:      dialChan,
	}
	go run(ctx, sdAddr, d, dialChan, onDialError, done)
	return &headerDialer{Listener: sd}, err
}

func run(
	ctx context.Context,
	addr serverDialAddr,
	d dialer,
	dialChan chan<- net.Conn,
	onDialError func(error),
	done chan<- struct{},
) {
	defer close(done)
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
			onDialError(fmt.Errorf("failed to dial %s: %w", addr.addr, err))
			continue
		}
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
	}
}

type wrappedConn struct {
	c         net.Conn
	closeOnce sync.Once
	onClose   chan<- struct{}
	closeErr  error
}

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

func (w *wrappedConn) Close() error {
	w.closeOnce.Do(func() {
		w.closeErr = w.c.Close()
		close(w.onClose)
	})
	return w.closeErr
}

var _ net.Conn = (*wrappedConn)(nil)

// Accept implements net.Listener.
func (s *Listener) Accept() (net.Conn, error) {
	select {
	case <-s.dialingCtx.Done():
		return nil, s.dialingCtx.Err()
	case conn := <-s.dialChan:
		return conn, nil
	}
}

// Addr implements net.Listener.
func (s *Listener) Addr() net.Addr {
	return &s.addr
}

type serverDialAddr struct {
	scheme string
	addr   string
}

type dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

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
	return d, serverDialAddr{
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
func (s *Listener) Close() error {
	s.cancelDialing()
	<-s.done
	return nil
}

var _ net.Listener = (*Listener)(nil)
