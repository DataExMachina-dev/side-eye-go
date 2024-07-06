// Package sideeye contains a library to snapshot data using the side-eye
// service.
package sideeye

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
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

// Explicitly set the environment label for this process.
//
// Defaults to SIDE_EYE_ENVIRONMENT if not set.
func WithEnvironment(env string) Option {
	return optionFunc(func(cfg *config) {
		cfg.environment = env
	})
}

// Explicitly set the API URL for this process.
//
// Defaults to SIDE_EYE_API_URL if not set.
func WithApiUrl(url string) Option {
	return optionFunc(func(cfg *config) {
		cfg.apiUrl = url
	})
}

// Explicitly set the API token for this process.
//
// Defaults to SIDE_EYE_TOKEN if not set.
func WithToken(token string) Option {
	return optionFunc(func(cfg *config) {
		cfg.tenantToken = token
	})
}

// Set a function to be called with errors.
func WithErrorLogger(f func(err error)) Option {
	return optionFunc(func(cfg *config) {
		cfg.errorLogger = f
	})
}

// Init initializes the Side-Eye library. A connection is established to the
// Side-Eye cloud service and this process is registered to be monitored.
func Init(
	ctx context.Context,
	opts ...Option,
) error {
	return singletonConn.Connect(ctx, opts...)
}

// Stop terminates the connection to the Side-Eye cloud service.
func Stop() {
	singletonConn.Close()
}

// singletonConn is the connection manipulated by Init() / Stop().
var singletonConn = sideEyeConn{}

type sideEyeConn struct {
	connected    bool
	activeConfig config
	server       *server.Server
	grpcServer   *grpc.Server
	grpcConn     *grpc.ClientConn

	wg *sync.WaitGroup
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
	c.connected = true
	c.activeConfig = cfg
	c.server = server
	c.grpcServer = s
	c.grpcConn = conn
	wg := &sync.WaitGroup{}
	wg.Add(1)
	c.wg = wg

	go func() {
		defer func() {
			c.connected = false
		}()
		defer wg.Done()
		defer c.closeInner()
		if err := s.Serve(l); err != nil {
			// TODO: Handle this error better.
			cfg.errorLogger(fmt.Errorf("failed to serve: %w", err))
		}
	}()
	return nil
}

// Close closes the connection. It's a no-op if the connection was never
// established.
func (c *sideEyeConn) Close() {
	if !c.connected {
		return
	}

	c.closeInner()

	// Synchronize with the goroutine handling RPCs.
	c.wg.Wait()
	c.connected = false
}

// closeInner closes the connection. Unlike Close(), it doesn't wait for the
// server goroutine to terminate.
func (c *sideEyeConn) closeInner() {
	if !c.connected {
		return
	}
	c.grpcServer.Stop()
	c.grpcConn.Close()
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

func HttpHandler() http.Handler {
	return httpHandler{}
}

type httpHandler struct{}

func (h httpHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// GETs render the current state of the connection.
	if req.Method == http.MethodGet {
		h.handleGet(w)
		return
	}

	// If this is a POST request, stop the old connection (if any) and, if a token
	// is specified, start a new connection using it.
	if err := req.ParseForm(); err != nil {
		singletonConn.activeConfig.errorLogger(fmt.Errorf("failed to parse form: %w", err))
		return
	}

	if _, ok := req.Form["disconnect"]; ok {
		Stop()
		h.handleGet(w)
		return
	}

	if _, ok := req.Form["connect"]; !ok {
		singletonConn.activeConfig.errorLogger(fmt.Errorf("invalid POST: missing connect/disconnect"))
		return
	}

	// Connect (or re-connect) with the new configuration.

	var newToken, newEnv string
	if tok, ok := req.Form["token"]; ok {
		newToken = tok[0]
	} else {
		singletonConn.activeConfig.errorLogger(fmt.Errorf("invalid POST: missing token"))
		return
	}
	if env, ok := req.Form["env"]; ok {
		newEnv = env[0]
	} else {
		singletonConn.activeConfig.errorLogger(fmt.Errorf("invalid POST: missing env"))
		return
	}

	// If there was a prior connection to Side-Eye, close it.
	Stop()

	// If we're configured with a token, start a new connection to Side-Eye.
	if newToken != "" {
		err := Init(context.Background(), WithToken(newToken), WithEnvironment(newEnv))
		if err != nil {
			singletonConn.activeConfig.errorLogger(fmt.Errorf("failed to update config: %w", err))
		}
	}

	// Generate the page after the update.
	h.handleGet(w)
}

func (h httpHandler) handleGet(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)

	var status, color string
	if singletonConn.connected {
		status = "connected"
		color = "green"
	} else {
		status = "disconnected"
		color = "red"
	}

	sb := strings.Builder{}
	sb.WriteString(`<html>
<head>
	<title>Side-Eye configuration</title>
	<style>
	.circle {
		height: 21px;
		width: 21px;
		border-radius: 50%;
		display: inline-block;
	}
	</style>
</head>
<body>
<h1>Side-Eye configuration</h1>
<form action="" method="POST">
<div style="
	display:grid;
	gap:3px;
	grid-template-columns: 9em 20em;
	margin-bottom: 10px;"
	>
`)
	sb.WriteString(fmt.Sprintf(`
<div>Connection status:</div>
<div style="display:flex; flex-direction:row; align-items:center; gap:3px">
	<div class="circle" style="background-color:%s;"></div>
	<span>%s</span>
</div>`, color, status))
	sb.WriteString("<div>Side-Eye token:</div>")
	sb.WriteString(fmt.Sprintf(`<input type="text" name="token" value="%s"/>`,
		singletonConn.activeConfig.tenantToken))
	sb.WriteString("<div>Environment:</div>")
	sb.WriteString(fmt.Sprintf(`<input type="text" name="env" value="%s"/>`,
		singletonConn.activeConfig.environment))

	disconnectAttribute := ""
	if !singletonConn.connected {
		disconnectAttribute = "disabled"
	}

	sb.WriteString(fmt.Sprintf(`
</div>
<input type="submit" value="Reconnect" name="connect"/>
<input type="submit" value="Disconnect" name="disconnect" %s/>
</form>
</body>
</html>`, disconnectAttribute))

	_, err := w.Write([]byte(sb.String()))
	if err != nil {
		singletonConn.activeConfig.errorLogger(fmt.Errorf("failed to write response: %w", err))
	}
	return
}
