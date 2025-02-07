package sideeyeconn

import (
	"context"
	"fmt"
	"github.com/DataExMachina-dev/side-eye-go/internal/artifactspb"
	"github.com/DataExMachina-dev/side-eye-go/internal/machinapb"
	"github.com/DataExMachina-dev/side-eye-go/internal/server"
	"github.com/DataExMachina-dev/side-eye-go/internal/serverdial"
	"github.com/DataExMachina-dev/side-eye-go/internal/stoptheworld"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"net/netip"
	"net/url"
	"os"
	"sync"
)

type Config struct {
	TenantToken string
	AgentUrl    string
	Environment string
	ProgramName string
	ErrorLogger func(err error)
}

const (
	defaultAgentUrl = "https://internal-api.side-eye.io:443"

	ENV_AGENT_URL    = "SIDE_EYE_AGENT_URL"
	ENV_TENANT_TOKEN = "SIDE_EYE_TOKEN"
	ENV_ENVIRONMENT  = "SIDE_EYE_ENVIRONMENT"
)

func MakeDefaultConfig(programName string) Config {
	cfg := Config{
		ProgramName: programName,
		AgentUrl:    defaultAgentUrl,
		ErrorLogger: func(err error) {},
	}
	if os.Getenv(ENV_TENANT_TOKEN) != "" {
		cfg.TenantToken = os.Getenv(ENV_TENANT_TOKEN)
	}
	if os.Getenv(ENV_AGENT_URL) != "" {
		cfg.AgentUrl = os.Getenv(ENV_AGENT_URL)
	}
	if os.Getenv(ENV_ENVIRONMENT) != "" {
		cfg.Environment = os.Getenv(ENV_ENVIRONMENT)
	}
	return cfg
}

// SideEyeConn represents a connection to the Side-Eye cloud service.
type SideEyeConn struct {
	ActiveConfig Config
	// agentFingerprint is the ID that this process identifies as to the Side-Eye
	// service when it connects as an agent.
	agentFingerprint uuid.UUID
	// processFingerprint is the process ID that will be reported to the Side-Eye
	// service.
	processFingerprint string

	// Fields that change in Connect/Close.
	mu struct {
		sync.Mutex
		// listener is the network listener for the gRPC server. The listener dials
		// one connection at a time to the Side-Eye service, and accepts requests on
		// that connection.
		listener *serverdial.Listener

		server     *server.Server
		grpcServer *grpc.Server
		grpcConn   *grpc.ClientConn
	}

	wg *sync.WaitGroup
}

func NewSideEyeConn() *SideEyeConn {
	return &SideEyeConn{
		ActiveConfig: Config{
			// no-op logger
			ErrorLogger: func(err error) {},
		},
	}
}

func (c *SideEyeConn) AgentFingerprint() uuid.UUID {
	return c.agentFingerprint
}
func (c *SideEyeConn) ProcessFingerprint() string {
	return c.processFingerprint
}

// Connect connects to the Side-Eye cloud service and registers this process to
// be monitored. A goroutine is started to handle incoming RPCs.
//
// programName is the name of the program that this process will correspond to
// on app.side-eye.io.
//
// ephemeralProcess indicates whether this process identifies itself as
// "ephemeral" to the Side-Eye service. Ephemeral processes are not visible in
// the Side-Eye UI; snapshots of ephemeral processes can only be captured
// programmatically.
//
// c.Close() should be called to stop monitoring the process.
func (c *SideEyeConn) Connect(
	ctx context.Context,
	cfg Config,
	ephemeralProcess bool,
) error {
	if err := stoptheworld.PlatformSupported(); err != nil {
		return err
	}

	// If we were already connected, terminate that connection.
	c.Close()

	if cfg.TenantToken == "" {
		return fmt.Errorf("missing token")
	}

	var err error
	c.agentFingerprint, err = uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("failed to generate fingerprint: %w", err)
	}
	ti := getStartTime()
	c.processFingerprint = fmt.Sprintf(
		"%s:%d:%d.%d",
		c.agentFingerprint.String(),
		os.Getpid(),
		ti.Unix(),
		ti.UnixNano()-ti.Unix()*1_000_000_000,
	)

	l, err := serverdial.NewListener(cfg.AgentUrl, cfg.ErrorLogger)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	s := grpc.NewServer()
	client, conn, err := newArtifactsClient(cfg.AgentUrl)
	if err != nil {
		return fmt.Errorf("failed to create artifacts client: %w", err)
	}
	fetcher := server.NewSnapshotFetcher(client)
	server := server.NewServer(
		c.agentFingerprint, c.processFingerprint,
		cfg.TenantToken, cfg.Environment, cfg.ProgramName, fetcher,
		ephemeralProcess)
	machinapb.RegisterMachinaServer(s, server)
	machinapb.RegisterGoPprofServer(s, server)
	c.ActiveConfig = cfg
	c.mu.Lock()
	c.mu.server = server
	c.mu.grpcServer = s
	c.mu.grpcConn = conn
	c.mu.listener = l
	c.mu.Unlock()
	wg := &sync.WaitGroup{}
	wg.Add(1)
	c.wg = wg

	go func() {
		defer wg.Done() // unblock Close()
		defer c.closeInner()
		if err := s.Serve(l); err != nil {
			// TODO: Handle this error better.
			cfg.ErrorLogger(fmt.Errorf("failed to serve: %w", err))
		}
	}()
	return nil
}

func (c *SideEyeConn) listener() *serverdial.Listener {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mu.listener
}

// Close closes the connection. It's a no-op if the connection was never
// established. Connect() can be called again after Close() to re-establish the
// connection.
func (c *SideEyeConn) Close() {
	if c.listener() == nil {
		return
	}

	c.closeInner()

	// Synchronize with the goroutine handling RPCs.
	c.wg.Wait()
}

// closeInner closes the connection. Unlike Close(), it doesn't wait for the
// server goroutine to terminate.
//
// closeInner might be called concurrently with Close().
func (c *SideEyeConn) closeInner() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.mu.listener == nil {
		// Connection has already been closed.
		return
	}
	c.mu.grpcServer.Stop()
	c.mu.grpcConn.Close()
	c.mu.grpcConn = nil
	c.mu.grpcServer = nil
	c.mu.server = nil
	c.mu.listener = nil
}

type ConnectionStatus int

const (
	UnknownStatus ConnectionStatus = iota
	// Uninitialized means Connect() was never called, or Close() was called.
	Uninitialized
	Connected
	Disconnected
	Connecting
)

func (c *SideEyeConn) Status() ConnectionStatus {
	l := c.listener()
	if l == nil {
		return Uninitialized
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
