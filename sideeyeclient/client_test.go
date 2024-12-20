package sideeyeclient_test

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/DataExMachina-dev/side-eye-go/sideeyeclient"
)

var environment = flag.String("env", "roachprod-andrew-test", "The environment to operate on.")

// Test that capturing a snapshot of the environment indicated by --env works.
//
// The SIDE_EYE_TOKEN env var needs to be set.
func TestCaptureSnapshot(t *testing.T) {
	if os.Getenv("SIDE_EYE_TOKEN") == "" {
		t.Skip("SIDE_EYE_TOKEN environment variable is not set")
	}
	c, err := sideeyeclient.NewSideEyeClient(sideeyeclient.WithApiTokenFromEnv{})
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}
	res, err := c.CaptureSnapshot(context.Background(), *environment)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot URL: %s", res.SnapshotURL)
}
