package apiclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/DataExMachina-dev/side-eye-go/internal/apipb"
)

const ENV_API_URL = "SIDE_EYE_API_URL"

type APIClient struct {
	conn     *grpc.ClientConn
	client   apipb.ApiServiceClient
	apiToken string
}

// NewAPIClient creates a new APIClient for talking to the Side-Eye service.
//
// Close() needs to be called on the client when it is no longer needed to
// release resources.
func NewAPIClient(apiToken string) (*APIClient, error) {
	sideEyeURL := "https://api.side-eye.io"
	if url, ok := os.LookupEnv(ENV_API_URL); ok {
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

	grpcClient, err := grpc.Dial(grpcAddress, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the Side-Eye agents service: %w", err)
	}
	client := apipb.NewApiServiceClient(grpcClient)
	return &APIClient{conn: grpcClient, client: client, apiToken: apiToken}, nil
}

// Close closes the client's network connection.
func (c *APIClient) Close() {
	_ /* err */ = c.conn.Close()
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

func (c *APIClient) CaptureSnapshot(
	ctx context.Context, req *apipb.CaptureSnapshotRequest,
) (SnapshotResult, error) {
	if c.apiToken != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "api-token", c.apiToken)
	}
	res, err := c.client.CaptureSnapshot(ctx, req)
	if err != nil {
		// Recognize some error details and turn them into typed errors.
		s, ok := status.FromError(err)
		if ok && len(s.Details()) > 0 {
			detail := s.Details()[0]
			if sErr, ok := detail.(*apipb.SnapshotError); ok {
				switch sErr.ErrorKind {
				case apipb.ErrorKind_BINARY_STRIPPED:
					return SnapshotResult{}, BinaryStrippedError{
						msg: sErr.Message,
					}
				case apipb.ErrorKind_NO_AGENTS:
					return SnapshotResult{}, NoAgentsError{}
				case apipb.ErrorKind_NO_PROCESSES:
					return SnapshotResult{}, NoProcessesError{}
				case apipb.ErrorKind_PROCESS_MISSING:
					return SnapshotResult{}, ProcessMissingError{}
				case apipb.ErrorKind_ENVIRONMENT_MISSING:
					return SnapshotResult{}, EnvMissingError{msg: sErr.Message}
				}
			}
		}
		switch s.Code() {
		case codes.Unavailable:
			return SnapshotResult{}, fmt.Errorf("failed to connect to Side-Eye API service: %w", err)
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
			Error:    pe.Message,
		})
	}
	return snapRes, nil
}

// ProcessError describes an error that occurred while capturing a snapshot for
// one process.
type ProcessError struct {
	Hostname string
	Program  string
	Pid      int
	Error    string
}

type ProcessMissingError struct{}

var _ error = ProcessMissingError{}

func (n ProcessMissingError) Error() string {
	return "process not found"
}

// NoProcessesError is returned when none of the agents are reporting any
// processes that Side-Eye is configured to monitor.
type NoProcessesError struct{}

var _ error = NoProcessesError{}

func (n NoProcessesError) Error() string {
	return "no matching processes found"
}

type BinaryStrippedError struct {
	msg string
}

var _ error = BinaryStrippedError{}

func (b BinaryStrippedError) Error() string {
	return b.msg
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
