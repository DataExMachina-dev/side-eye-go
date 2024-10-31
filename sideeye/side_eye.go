// Package sideeye contains a library to snapshot data using the side-eye
// service.
package sideeye

import (
	"context"
	"fmt"
	"github.com/DataExMachina-dev/side-eye-go/internal/apiclient"
	"github.com/DataExMachina-dev/side-eye-go/internal/apipb"
	"github.com/DataExMachina-dev/side-eye-go/internal/sideeyeconn"
)

// ENV_AGENT_URL is the environment variable that overrides the URL to which
// side-eye-go connects to as an agent.
const ENV_AGENT_URL = sideeyeconn.ENV_AGENT_URL

// Option to configure the Side-Eye library.
type Option interface {
	apply(*sideeyeconn.Config)
}

func makeConfig(programName string, opts ...Option) sideeyeconn.Config {
	cfg := sideeyeconn.MakeDefaultConfig(programName)
	for _, opt := range opts {
		opt.apply(&cfg)
	}
	return cfg
}

type optionFunc func(cfg *sideeyeconn.Config)

func (f optionFunc) apply(cfg *sideeyeconn.Config) {
	f(cfg)
}

// WithToken sets the API token to use for authenticating to Side-Eye. Defaults
// to the SIDE_EYE_TOKEN environment variable if this option is not used.
//
// To get your organization's token, log in app.side-eye.io.
func WithToken(token string) Option {
	return optionFunc(func(cfg *sideeyeconn.Config) {
		cfg.TenantToken = token
	})
}

// WithEnvironment sets the environment label for this process. Defaults to the
// SIDE_EYE_ENVIRONMENT environment variable if this option is not used.
//
// If this option is not used and SIDE_EYE_ENVIRONMENT is not set, this process
// will still be monitored by Side-Eye but it will not be part of any named
// environment.
func WithEnvironment(env string) Option {
	return optionFunc(func(cfg *sideeyeconn.Config) {
		cfg.Environment = env
	})
}

func WithProgramName(programName string) Option {
	return optionFunc(func(cfg *sideeyeconn.Config) {
		cfg.ProgramName = programName
	})
}

// WithErrorLogger sets a function to be called with errors (for example for
// logging them).
func WithErrorLogger(f func(err error)) Option {
	return optionFunc(func(cfg *sideeyeconn.Config) {
		cfg.ErrorLogger = f
	})
}

// Init initializes the Side-Eye library. A connection is established to the
// Side-Eye cloud service and this process is registered to be monitored -- i.e.
// this process shows up on app.side-eye.io and can be selected to be included
// in snapshots or event traces.
//
// programName is the name of the program that this process will be identified
// as on app.side-eye.io.
func Init(
	ctx context.Context,
	programName string,
	opts ...Option,
) error {
	cfg := sideeyeconn.MakeDefaultConfig(programName)
	for _, opt := range opts {
		opt.apply(&cfg)
	}
	if err := singletonConn.Connect(ctx, cfg, false /* ephemeralProcess */); err != nil {
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
var singletonConn = sideeyeconn.NewSideEyeConn()

// CaptureSelfSnapshot captures a snapshot of the current process.
//
// If ctx has a timeout/deadline/cancellation, CaptureSelfSnapshot will return
// when the context is canceled.
//
// It is equivalent to:
// ```
// Init(WithEnvironment(<unique name>), WithProgramName("myProgram"))
// defer Stop()
// sideeyeclient.CaptureSnapshot(ctx, <unique name>)
// ```
// with the following differences:
//   - The current process never appears in the list of processes seen on
//     https://app.side-eye.io.
//   - CaptureSelfSnapshot() deals with the race between the process
//     being registered and the snapshot being captured.
func CaptureSelfSnapshot(
	ctx context.Context, programName string, opts ...Option,
) (string, error) {
	// Connect to the Side-Eye service as a monitored process in "ephemeral" mode.
	cfg := makeConfig(programName, opts...)
	conn := sideeyeconn.NewSideEyeConn()
	if err := conn.Connect(ctx, cfg, true /* ephemeralProcess */); err != nil {
		return "", fmt.Errorf("failed to connect to Side-Eye: %w", err)
	}
	defer conn.Close()

	// Connect to the Side-Eye API and ask for a snapshot of the current process.
	apiClient, err := apiclient.NewAPIClient(cfg.TenantToken)
	if err != nil {
		return "", fmt.Errorf("failed to create Side-Eye API client: %w", err)
	}
	defer apiClient.Close()
	res, err := apiClient.CaptureSnapshot(ctx,
		&apipb.CaptureSnapshotRequest{
			AgentFingerprint:   conn.AgentFingerprint().String(),
			ProcessFingerprint: conn.ProcessFingerprint(),
		})
	if err != nil {
		return "", err
	}
	return res.SnapshotURL, nil
}

type BinaryStrippedError = apiclient.BinaryStrippedError
