package side_eye_client_go

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"os"

	"github.com/DataExMachina-dev/side-eye-client-go/apipb"
)

type SnapshotResult struct {
	SnapshotURL   string
	ProcessErrors []string
}

type SideEyeClient struct {
	conn   *grpc.ClientConn
	client apipb.ApiServiceClient
	opts   sideEyeClientOpts
}

func NewSideEyeClient(option ...SideEyeClientOption) (*SideEyeClient, error) {
	opts := sideEyeClientOpts{}
	for _, o := range option {
		o.apply(&opts)
	}
	// !!!
	grpcClient, err := grpc.Dial("127.0.0.1:12346")
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
	return SnapshotResult{
		SnapshotURL:   res.SnapshotUrl,
		ProcessErrors: res.Errors,
	}, nil
}
