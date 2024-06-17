// Package sideeye contains a library to snapshot data using the side-eye
// service.
package sideeye

import (
	"context"
	"fmt"
	"net/netip"
	"net/url"
	"os"

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

// Initialize the side-eye library.
func Init(
	ctx context.Context,
	opts ...Option,
) error {
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
	client, err := newArtifactsClient(cfg.apiUrl)
	if err != nil {
		return fmt.Errorf("failed to create artifacts client: %w", err)
	}
	fetcher := server.NewSnapshotFetcher(client)
	server := server.NewServer(fingerprint, cfg.tenantToken, cfg.environment, fetcher)
	machinapb.RegisterMachinaServer(s, server)

	go func() {
		if err := s.Serve(l); err != nil {
			// TODO: Handle this error better.
			cfg.errorLogger(fmt.Errorf("failed to serve: %w", err))
		}
	}()
	return nil
}

func newArtifactsClient(
	addr string,
) (artifactspb.ArtifactStoreClient, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}
	var opts []grpc.DialOption
	switch u.Scheme {
	case "http":
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	case "https":
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(nil)))
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", u.Scheme)
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

	client, err := grpc.DialContext(context.Background(), addr, opts...)
	if err != nil {
		return nil, err
	}
	return artifactspb.NewArtifactStoreClient(client), nil
}
