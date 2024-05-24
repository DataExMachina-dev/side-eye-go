package side_eye_client_go

import (
	"context"
	"crypto/tls"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/DataExMachina-dev/side-eye-client-go/apipb"
)

// SideEyeClient is a client for the Side-Eye service.
type SideEyeClient struct {
	conn   *grpc.ClientConn
	client apipb.ApiServiceClient
	opts   sideEyeClientOpts
}

// NewSideEyeClient creates a new SideEyeClient. WithApiToken or
// WithApiTokenFromEnv need to specified as an option to authenticate with the
// Side-Eye service.
//
// Close() needs to be called on the client when it is no longer needed to
// release resources.
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

// Close closes the client's network connection.
func (c *SideEyeClient) Close() {
	_ /* err */ = c.conn.Close()
}

type sideEyeClientOpts struct {
	apiToken string
}

// SideEyeClientOption is the interface implemented by options for
// NewSideEyeClient.
type SideEyeClientOption interface {
	apply(*sideEyeClientOpts) error
}

// WithApiToken is a string option for NewSideEyeClient that specifies the API
// token to use for authentication to the Side-Eye service.
type WithApiToken string

var _ SideEyeClientOption = WithApiToken("")

// apply implements the SideEyeClientOption interface.
func (t WithApiToken) apply(opts *sideEyeClientOpts) error {
	opts.apiToken = string(t)
	return nil
}

// WithApiTokenFromEnv is an option for NewSideEyeClient that specifies that the
// API token to use for authentication to the Side-Eye service should be read
// from the SIDEEYE_TOKEN environment variable. If that variable is not set,
// NewSideEyeClient will return an error.
type WithApiTokenFromEnv struct{}

var _ SideEyeClientOption = WithApiTokenFromEnv{}

// apply implements the SideEyeClientOption interface.
func (w WithApiTokenFromEnv) apply(opts *sideEyeClientOpts) error {
	tok, ok := os.LookupEnv("SIDEEYE_TOKEN")
	if !ok {
		return fmt.Errorf("SIDEEYE_API_TOKEN environment variable required by WithApiTokenFromEnv is not set")
	}
	opts.apiToken = tok
	return nil
}

// CaptureSnapshot captures a snapshot of all the monitored processes in the
// requested environment. If envName is empty, it refers to all the processes
// reported by agents not configured with an environment.
//
// Besides generic errors, CaptureSnapshot can return NoAgentsError,
// EnvMissingError or NoProcessesError.
func (c *SideEyeClient) CaptureSnapshot(
	ctx context.Context, envName string,
) (SnapshotResult, error) {
	if c.opts.apiToken != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "api-token", c.opts.apiToken)
	}
	res, err := c.client.CaptureSnapshot(ctx, &apipb.CaptureSnapshotRequest{Environment: envName})
	if err != nil {
		s, ok := status.FromError(err)
		if ok {
			switch s.Code() {
			case codes.NotFound:
				return SnapshotResult{}, NoProcessesError{}
			case codes.FailedPrecondition:
				if strings.Contains(s.Message(), "no agents connected") {
					return SnapshotResult{}, NoAgentsError{}
				}
				if strings.Contains(s.Message(), "no agents found for environment") {
					return SnapshotResult{}, EnvMissingError{msg: s.Message()}
				}
			}
		}
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
	return snapRes, nil
}

// SnapshotResult describes the result of a successful snapshot capture.
type SnapshotResult struct {
	// SnapshotURL is the URL at which the snapshot can be accessed.
	SnapshotURL string
	// ProcessErrors, if not empty, contains info on the processes that Side-Eye
	// failed to include in the snapshot. Note that at least one process must have
	// been successful, otherwise CaptureSnapshot() would have returned an error.
	ProcessErrors []ProcessError
}

// ProcessError describes an error that occurred while capturing a snapshot for
// one process.
type ProcessError struct {
	Hostname string
	Program  string
	Pid      int
	Error    string
}

// NoProcessesError is returned when none of the agents are reporting any
// processes that Side-Eye is configured to monitor.
type NoProcessesError struct{}

var _ error = NoProcessesError{}

func (n NoProcessesError) Error() string {
	return "no matching processes found"
}

// NoAgentsError is returned when no agents are connected to Side-Eye (with a
// matching API token).
type NoAgentsError struct{}

var _ error = NoAgentsError{}

func (n NoAgentsError) Error() string {
	return "no agents connected"
}

// EnvMissingError is returned when no connected agents are configured with the
// requested environment.
type EnvMissingError struct {
	msg string
}

var _ error = EnvMissingError{}

func (e EnvMissingError) Error() string {
	return e.msg
}
