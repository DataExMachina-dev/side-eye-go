package side_eye_client_go

import (
	"context"
	"crypto/tls"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"net"
	"net/url"
	"os"

	"github.com/DataExMachina-dev/side-eye-client-go/apipb"
)

type SnapshotResult struct {
	SnapshotURL   string
	ProcessErrors []ProcessError
}

type ProcessError struct {
	Hostname string
	Program  string
	Pid      int
	Error    string
}

type SideEyeClient struct {
	conn   *grpc.ClientConn
	client apipb.ApiServiceClient
	opts   sideEyeClientOpts
}

func NewSideEyeClient(option ...SideEyeClientOption) (*SideEyeClient, error) {
	sideEyeURL := "https://api.side-eye.io"
	if url, ok := os.LookupEnv("SIDEEYE_URL"); ok {
		sideEyeURL = url
	}
	// Turn the URL into a gRPC address.
	parsed, err := url.Parse(sideEyeURL)
	if err != nil {
		return nil, err
	}
	var grpcAddress string
	var dialOpts []grpc.DialOption
	switch parsed.Scheme {
	case "http":
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		ip := net.ParseIP(parsed.Hostname())
		if ip != nil && parsed.Port() != "" {
			grpcAddress = net.JoinHostPort(ip.String(), parsed.Port())
		} else if ip != nil {
			grpcAddress = ip.String()
		} else {
			grpcAddress = fmt.Sprintf("dns:///%s", parsed.Host)
		}
	case "https":
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
		grpcAddress = fmt.Sprintf("dns:///%s", parsed.Host)
	default:
	}

	opts := sideEyeClientOpts{}
	for _, o := range option {
		o.apply(&opts)
	}
	grpcClient, err := grpc.Dial(grpcAddress, dialOpts...)
	if err != nil {
		return nil, err
	}
	client := apipb.NewApiServiceClient(grpcClient)
	return &SideEyeClient{conn: grpcClient, client: client, opts: opts}, nil
}

type sideEyeClientOpts struct {
	apiToken string
}

type SideEyeClientOption interface {
	apply(*sideEyeClientOpts) error
}

type WithApiToken string

var _ SideEyeClientOption = WithApiToken("")

func (t WithApiToken) apply(opts *sideEyeClientOpts) error {
	opts.apiToken = string(t)
	return nil
}

type WithApiTokenFromEnv struct{}

var _ SideEyeClientOption = WithApiTokenFromEnv{}

func (w WithApiTokenFromEnv) apply(opts *sideEyeClientOpts) error {
	tok, ok := os.LookupEnv("SIDEEYE_API_TOKEN")
	if !ok {
		return fmt.Errorf("SIDEEYE_API_TOKEN environment variable required by WithApiTokenFromEnv is not set")
	}
	opts.apiToken = tok
	return nil
}

func (c *SideEyeClient) Close() {
	_ /* err */ = c.conn.Close()
}

func (c *SideEyeClient) CaptureSnapshot(
	ctx context.Context, envName string,
) (SnapshotResult, error) {
	if c.opts.apiToken == "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "api-token", c.opts.apiToken)
	}
	res, err := c.client.CaptureSnapshot(ctx, &apipb.CaptureSnapshotRequest{Environment: envName})
	if err != nil {
		return SnapshotResult{}, err
	}

	// Convert the gRPC response into our format.
	snapRes := SnapshotResult{
		SnapshotURL: res.SnapshotUrl,
	}
	for _, pe := range res.Errors {
		snapRes.ProcessErrors = append(snapRes.ProcessErrors, ProcessError{
			Hostname: pe.Hostname,
			Program:  pe.Program,
			Pid:      int(pe.Pid),
			Error:    pe.Error,
		})
	}
	// !!! Handle the error when no processes were found.
	return snapRes, nil
}
