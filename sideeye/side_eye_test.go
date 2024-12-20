package sideeye_test

import (
	"context"
	"github.com/DataExMachina-dev/side-eye-go/sideeye"
	"log"
	"os"
	"testing"
)

// Test that capturing a snapshot works.
//
// The SIDE_EYE_TOKEN env var needs to be set.
// Also, `go test` strips the test binary from debug info, which will cause the
// snapshot to fail. Running with `go test -o` works:
// SIDE_EYE_API_URL=http://localhost:12347 SIDE_EYE_AGENT_URL=http://localhost:12345 SIDE_EYE_TOKEN=<your token> go test -o test.out -test.v
func TestCaptureSelfSnapshot(t *testing.T) {
	if os.Getenv("SIDE_EYE_TOKEN") == "" {
		t.Skip("SIDE_EYE_TOKEN environment variable is not set")
	}

	url, err := sideeye.CaptureSelfSnapshot(
		context.Background(),
		"testProgram",
		sideeye.WithErrorLogger(func(err error) {
			log.Printf("Side-Eye error: %s", err)
		}))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("snapshot URL: %s", url)
}
