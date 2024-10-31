package sideeyeclient

import (
	"context"
	"fmt"
	"github.com/DataExMachina-dev/side-eye-go/internal/apiclient"
	"github.com/DataExMachina-dev/side-eye-go/internal/apipb"
	"net/url"
	"os"
)

// SideEyeClient is a client for the Side-Eye service.
type SideEyeClient struct {
	client *apiclient.APIClient
}

const (
	ENV_TENANT_TOKEN = "SIDE_EYE_TOKEN"
)

// NewSideEyeClient creates a new SideEyeClient. WithApiToken or
// WithApiTokenFromEnv need to specified as an option to authenticate with the
// Side-Eye service.
//
// Close() needs to be called on the client when it is no longer needed to
// release resources.
func NewSideEyeClient(option ...SideEyeClientOption) (*SideEyeClient, error) {
	opts := sideEyeClientOpts{}
	for _, o := range option {
		o.apply(&opts)
	}
	innerClient, err := apiclient.NewAPIClient(opts.apiToken)
	if err != nil {
		return nil, err
	}
	return &SideEyeClient{
		client: innerClient,
	}, nil
}

// Close closes the client's network connection.
func (c *SideEyeClient) Close() {
	c.client.Close()
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
// from the SIDE_EYE_TOKEN environment variable. If that variable is not set,
// NewSideEyeClient will return an error.
type WithApiTokenFromEnv struct{}

var _ SideEyeClientOption = WithApiTokenFromEnv{}

// apply implements the SideEyeClientOption interface.
func (w WithApiTokenFromEnv) apply(opts *sideEyeClientOpts) error {
	tok, ok := os.LookupEnv(ENV_TENANT_TOKEN)
	if !ok {
		return fmt.Errorf("SIDEEYE_API_TOKEN environment variable required by WithApiTokenFromEnv is not set")
	}
	opts.apiToken = tok
	return nil
}

type SnapshotResult = apiclient.SnapshotResult

// CaptureSnapshot captures a snapshot of all the monitored processes in the
// requested environment. If envName is empty, it refers to all the processes
// reported by agents not configured with an environment.
//
// Besides generic errors, CaptureSnapshot can return NoAgentsError,
// EnvMissingError or NoProcessesError.
func (c *SideEyeClient) CaptureSnapshot(
	ctx context.Context, envName string,
) (SnapshotResult, error) {
	request := &apipb.CaptureSnapshotRequest{Environment: envName}
	return c.client.CaptureSnapshot(ctx, request)
}

// RecordingsURL returns the URL of the Side-Eye app's recordings page. If an
// environment is specified, the URL will point to a view filtered to that
// environment's snapshots. If environment is empty, the URL will point to all
// snapshots.
func RecordingsURL(environment string) string {
	if environment == "" {
		return "https://app.side-eye.io/#/recordings"
	}
	return fmt.Sprintf("https://app.side-eye.io/#/recordings?env=%s", url.QueryEscape(environment))
}
