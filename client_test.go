package side_eye_client_go_test

import (
	"context"
	"flag"
	"testing"

	side_eye_client_go "github.com/DataExMachina-dev/side-eye-client-go"
)

var environment = flag.String("env", "roachprod-andrew-test", "The environment to operate on.")

// Test that capturing a snapshot works.
//
// The SIDEEYE_TOKEN env var needs to be set.
func TestCaptureSnapshot(t *testing.T) {
	c, err := side_eye_client_go.NewSideEyeClient(side_eye_client_go.WithApiTokenFromEnv{})
	if err != nil {
		t.Fatalf("failed to create client: %s", err)
	}
	res, err := c.CaptureSnapshot(context.Background(), *environment)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot URL: %s", res.SnapshotURL)
}
