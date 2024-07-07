// Package sideeye contains a library to snapshot data using the side-eye
// service.
package sideeye

import (
	"context"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"sync"

	"github.com/DataExMachina-dev/side-eye-go/internal/artifactspb"
	"github.com/DataExMachina-dev/side-eye-go/internal/machinapb"
	"github.com/DataExMachina-dev/side-eye-go/internal/server"
	"github.com/DataExMachina-dev/side-eye-go/internal/serverdial"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Option to configure the side-eye library.
type Option interface {
	apply(*config)
}

type config struct {
	tenantToken string
	apiUrl      string
	environment string
	errorLogger func(err error)
}

const (
	defaultApiUrl = "https://internal-api.side-eye.io:443"

	ENV_API_URL      = "SIDE_EYE_API_URL"
	ENV_TENANT_TOKEN = "SIDE_EYE_TOKEN"
	ENV_ENVIRONMENT  = "SIDE_EYE_ENVIRONMENT"
)

func makeDefaultConfig() config {
	cfg := config{
		apiUrl:      defaultApiUrl,
		errorLogger: func(err error) {},
	}
	if os.Getenv(ENV_TENANT_TOKEN) != "" {
		cfg.tenantToken = os.Getenv(ENV_TENANT_TOKEN)
	}
	if os.Getenv(ENV_API_URL) != "" {
		cfg.apiUrl = os.Getenv(ENV_API_URL)
	}
	if os.Getenv(ENV_ENVIRONMENT) != "" {
		cfg.environment = os.Getenv(ENV_ENVIRONMENT)
	}
	return cfg
}

type optionFunc func(cfg *config)

func (f optionFunc) apply(cfg *config) {
	f(cfg)
}

// WithToken sets the API token to use for authenticating to Side-Eye. Defaults
// to the SIDE_EYE_TOKEN environment variable if this option is not used.
//
// To get your organization's token, log in app.side-eye.io.
func WithToken(token string) Option {
	return optionFunc(func(cfg *config) {
		cfg.tenantToken = token
	})
}

// WithEnvironment sets the environment label for this process. Defaults to the
// SIDE_EYE_ENVIRONMENT environment variable if this option is not used.
//
// If this option is not used and SIDE_EYE_ENVIRONMENT is not set, this process
// will still be monitored by Side-Eye but it will not be part of any named
// environment.
func WithEnvironment(env string) Option {
	return optionFunc(func(cfg *config) {
		cfg.environment = env
	})
}

// WithErrorLogger sets a function to be called with errors (for example for
// logging them).
func WithErrorLogger(f func(err error)) Option {
	return optionFunc(func(cfg *config) {
		cfg.errorLogger = f
	})
}

// Init initializes the Side-Eye library. A connection is established to the
// Side-Eye cloud service and this process is registered to be monitored -- i.e.
// this process shows up on app.side-eye.io and can be selected to be included
// in snapshots or event traces.
func Init(
	ctx context.Context,
	opts ...Option,
) error {
	if err := singletonConn.Connect(ctx, opts...); err != nil {
		return fmt.Errorf("failed to connect to Side-Eye: %w", err)
	}
	return nil
}

// Stop terminates the connection to the Side-Eye cloud service. It is a no-op
// if Init() hasn't been called. Init() can be called again after Stop() to
// re-establish the connection.
func Stop() {
	singletonConn.Close()
}

// singletonConn is the connection manipulated by Init() / Stop().
var singletonConn = newSideEyeConn()

// sideEyeConn represents a connection to the Side-Eye cloud service.
type sideEyeConn struct {
	activeConfig config
	server       *server.Server
	grpcServer   *grpc.Server
	grpcConn     *grpc.ClientConn

	// Fields that change in Connect/Close
	mu struct {
		sync.Mutex
		listener *serverdial.Listener
	}

	wg *sync.WaitGroup
}

func newSideEyeConn() *sideEyeConn {
	return &sideEyeConn{
		activeConfig: config{
			// no-op logger
			errorLogger: func(err error) {},
		},
	}
}

// Connect connects to the Side-Eye cloud service and registers this process to
// be monitored. A goroutine is started to handle incoming RPCs.
func (c *sideEyeConn) Connect(
	ctx context.Context,
	opts ...Option,
) error {
	// If we were already connected, terminate that connection.
	c.Close()

	cfg := makeDefaultConfig()
	for _, opt := range opts {
		opt.apply(&cfg)
	}

	if cfg.tenantToken == "" {
		return fmt.Errorf("missing token")
	}

	fingerprint, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("failed to generate fingerprint: %w", err)
	}

	l, err := serverdial.NewListener(cfg.apiUrl, cfg.errorLogger)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	s := grpc.NewServer()
	client, conn, err := newArtifactsClient(cfg.apiUrl)
	if err != nil {
		return fmt.Errorf("failed to create artifacts client: %w", err)
	}
	fetcher := server.NewSnapshotFetcher(client)
	server := server.NewServer(fingerprint, cfg.tenantToken, cfg.environment, fetcher)
	machinapb.RegisterMachinaServer(s, server)
	c.activeConfig = cfg
	c.server = server
	c.grpcServer = s
	c.grpcConn = conn
	c.setListener(l)
	wg := &sync.WaitGroup{}
	wg.Add(1)
	c.wg = wg

	go func() {
		defer wg.Done()
		defer c.closeInner()
		if err := s.Serve(l); err != nil {
			// TODO: Handle this error better.
			cfg.errorLogger(fmt.Errorf("failed to serve: %w", err))
		}
	}()
	return nil
}

func (c *sideEyeConn) setListener(l *serverdial.Listener) {
	c.mu.Lock()
	c.mu.listener = l
	c.mu.Unlock()
}

func (c *sideEyeConn) listener() *serverdial.Listener {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mu.listener
}

// Close closes the connection. It's a no-op if the connection was never
// established. Connect() can be called again after Close() to re-establish the
// connection.
func (c *sideEyeConn) Close() {
	if c.listener() == nil {
		return
	}

	c.closeInner()

	// Synchronize with the goroutine handling RPCs.
	c.wg.Wait()
}

// closeInner closes the connection. Unlike Close(), it doesn't wait for the
// server goroutine to terminate.
func (c *sideEyeConn) closeInner() {
	if c.listener == nil {
		return
	}
	c.grpcServer.Stop()
	c.grpcConn.Close()
	c.grpcConn = nil
	c.grpcServer = nil
	c.server = nil
	c.setListener(nil)
}

type ConnectionStatus int

const (
	UnknownStatus ConnectionStatus = iota
	Connected
	Disconnected
	Connecting
)

func (c *sideEyeConn) Status() ConnectionStatus {
	l := c.listener()
	if l == nil {
		return Disconnected
	}
	switch s := l.ConnectionStatus(); s {
	case serverdial.UnknownStatus:
		return UnknownStatus
	case serverdial.Connecting:
		return Connecting
	case serverdial.Connected:
		return Connected
	case serverdial.Disconnected:
		return Disconnected
	default:
		panic(fmt.Sprintf("unexpected status: %v", s))
	}
}

func newArtifactsClient(
	addr string,
) (artifactspb.ArtifactStoreClient, *grpc.ClientConn, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse url: %w", err)
	}
	var opts []grpc.DialOption
	switch u.Scheme {
	case "http":
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	case "https":
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(nil)))
	default:
		return nil, nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil {
		if ip.Is4() {
			addr = ip.String()
		} else {
			addr = ip.String()
		}
		if u.Port() != "" {
			addr = fmt.Sprintf("%s:%s", addr, u.Port())
		}
	} else {
		addr = fmt.Sprintf("dns:///%s", u.Host)
	}

	conn, err := grpc.DialContext(context.Background(), addr, opts...)
	if err != nil {
		return nil, nil, err
	}
	return artifactspb.NewArtifactStoreClient(conn), conn, nil
}
