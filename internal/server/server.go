package server

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strconv"
	"sync"

	"github.com/DataExMachina-dev/side-eye-go/internal/chunkpb"
	"github.com/DataExMachina-dev/side-eye-go/internal/machinapb"
	"github.com/DataExMachina-dev/side-eye-go/internal/snapshot"

	"github.com/google/uuid"
	"github.com/minio/highwayhash"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Server implements the machinapb.MachinaServer interface.
type Server struct {
	fingerprint uuid.UUID
	tenantToken string
	environment string
	fetcher     SnapshotFetcher
	hash        binaryHashOnce

	machinapb.UnimplementedMachinaServer
}

var _ machinapb.MachinaServer = (*Server)(nil)

type binaryHashOnce struct {
	sync.Once
	hash string
	err  error
}

// NewServer constructs a new Server object.
func NewServer(
	fingerprint uuid.UUID,
	tenantToken string,
	environment string,
	fetcher SnapshotFetcher,
) *Server {
	return &Server{
		fingerprint: fingerprint,
		tenantToken: tenantToken,
		environment: environment,
		fetcher:     fetcher,
	}
}

// GetExecutable implements machinapb.MachinaServer.
func (s *Server) GetExecutable(req *machinapb.GetExecutableRequest, stream machinapb.Machina_GetExecutableServer) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	exeFile, err := os.Open(exe)
	if err != nil {
		return fmt.Errorf("failed to open executable at %s: %w", exe, err)
	}
	const chunkSize = 128 << 10
	chunk := chunkpb.Chunk{
		Data: make([]byte, chunkSize),
	}
	for {
		n, err := exeFile.Read(chunk.Data[:chunkSize])
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read executable from %s: %w", exe, err)
		}
		chunk.Data = chunk.Data[:n]
		if err := stream.Send(&chunk); err != nil {
			return fmt.Errorf("failed to send executable: %w", err)
		}
	}
	return nil
}

// MachinaInfo implements machinapb.MachinaServer.
func (s *Server) MachinaInfo(req *machinapb.MachinaInfoRequest, stream machinapb.Machina_MachinaInfoServer) error {
	ctx := stream.Context()
	// TODO: Populate the rest of the fields.For version, perhaps
	// runtime.debug.ReadBuildInfo() is the ticket.
	if err := stream.Send(&machinapb.MachinaInfoResponse{
		Fingerprint: s.fingerprint.String(),
		Hostname:    "",
		Version:     "",
		TenantToken: s.tenantToken,
		Environment: s.environment,
		IpAddresses: []string{},
	}); err != nil {
		return fmt.Errorf("failed to send MachinaInfo: %w", err)
	}

	<-ctx.Done()
	return ctx.Err()
}

// Snapshot implements machinapb.MachinaServer.
func (s *Server) Snapshot(stream machinapb.Machina_SnapshotServer) (err error) {
	ctx := stream.Context()
	msg, err := stream.Recv()
	if err != nil {
		return fmt.Errorf("failed to receive SnapshotRequest: %w", err)
	}
	setupReq, ok := msg.Request.(*machinapb.SnapshotRequest_Setup_)
	if !ok {
		return fmt.Errorf("expected SnapshotRequest_Setup_ but got %T", msg.Request)
	}
	req, err := s.fetcher.FetchSnapshotProgram(ctx, setupReq.Setup.Key)
	if err != nil {
		return fmt.Errorf("failed to fetch snapshot program: %w", err)
	}
	if err := stream.SendHeader(nil); err != nil {
		return fmt.Errorf("failed to send header: %w", err)
	}
	msg, err = stream.Recv()
	if err != nil {
		return fmt.Errorf("failed to receive SnapshotRequest: %w", err)
	}
	if _, ok := msg.Request.(*machinapb.SnapshotRequest_Snapshot_); !ok {
		return fmt.Errorf("expected SnapshotRequest_Snapshot_ but got %T", msg.Request)
	}
	output, err := snapshot.Snapshot(req)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to snapshot: %v", err)
	}
	if err := stream.Send(output); err != nil {
		return fmt.Errorf("failed to send SnapshotResponse: %w", err)
	}
	return nil
}

// WatchProcesses implements machinapb.MachinaServer.
func (s *Server) WatchProcesses(r *machinapb.WatchProcessesRequest, watchServer machinapb.Machina_WatchProcessesServer) error {
	ctx := watchServer.Context()
	hash, err := s.getBinaryHash()
	if err != nil {
		return fmt.Errorf("failed to get binary hash: %w", err)
	}
	ti, err := getStartTime()
	if err != nil {
		return fmt.Errorf("failed to get start time: %w", err)
	}
	fingerprint := fmt.Sprintf(
		"%s:%d:%d.%d",
		s.fingerprint.String(),
		os.Getpid(),
		ti.Unix(),
		ti.UnixNano()-ti.Unix()*1_000_000_000,
	)
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	process := &machinapb.Process{
		Pid:         uint64(os.Getpid()),
		Cmd:         os.Args,
		ExePath:     exePath,
		Env:         os.Environ(),
		StartTime:   &timestamppb.Timestamp{},
		BinaryHash:  hash,
		Fingerprint: fingerprint,
		Environment: s.environment,
		Labels:      []*machinapb.LabelValue{},
	}
	classify(ctx, process, r.LabelRules)

	if err := watchServer.Send(&machinapb.Update{
		Added: []*machinapb.Process{process},
	}); err != nil {
		return fmt.Errorf("failed to send Update: %w", err)
	}
	<-ctx.Done()
	return ctx.Err()
}

func classify(ctx context.Context, p *machinapb.Process, rules []*machinapb.LabelRule) {
	matchRegexp := func(pat string, input string) bool {
		matched, err := regexp.MatchString(pat, input)
		return matched && err == nil
	}
	matchAnyRegexp := func(pats string, inputs []string) bool {
		re, err := regexp.Compile(pats)
		if err != nil {
			return false
		}
		for _, input := range inputs {
			if re.MatchString(input) {
				return true
			}
		}
		return false
	}

outer:
	for _, r := range rules {
		for _, c := range r.PredicatesConjunction {
			l, ok := machinapb.StandardLabels_value[c.Label]
			if !ok {
				continue outer
			}
			var matched bool
			switch machinapb.StandardLabels(l) {

			case machinapb.StandardLabels_executable_path:
				matched = matchRegexp(c.ValueRegex, p.ExePath)

				// The last element of executable_path.
			case machinapb.StandardLabels_executable_name:
				binaryName := path.Base(p.ExePath)
				matched = matchRegexp(c.ValueRegex, binaryName)

				// A filter on the command line passes if any of the individual arguments
				// match the regex.
			case machinapb.StandardLabels_command_line:
				matched = matchAnyRegexp(c.ValueRegex, p.Cmd)

				// A filter on the environment passes if any of the environment variable
				// key-values match the regex.
			case machinapb.StandardLabels_environment_variables:
				matched = matchAnyRegexp(c.ValueRegex, p.Env)

			case machinapb.StandardLabels_hostname:
				// TODO: add hostname filter

			case machinapb.StandardLabels_pid:
				matched = matchRegexp(c.ValueRegex, strconv.Itoa(int(p.Pid)))

			case machinapb.StandardLabels_program:
				matched = matchRegexp(c.ValueRegex, p.Program)

				// environment is a label that an agent can be configured to apply to the
				// processes it discovers.
			case machinapb.StandardLabels_environment:
				matched = matchRegexp(c.ValueRegex, p.Environment)

			default:
			}
			if !matched {
				continue outer
			}
		}

		outputLabel, ok := machinapb.StandardLabels_value[r.Label]
		if !ok {
			continue
		}
		switch machinapb.StandardLabels(outputLabel) {
		case machinapb.StandardLabels_program:
			p.Program = r.Value
		default:
		}
	}
}

func (s *Server) getBinaryHash() (string, error) {
	s.hash.Once.Do(func() {
		s.hash.hash, s.hash.err = doHash()
	})
	return s.hash.hash, s.hash.err
}

var hashKey = [32]byte{}

func doHash() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	exeFile, err := os.Open(exe)
	if err != nil {
		return "", fmt.Errorf("failed to open executable file at %s: %w", exe, err)
	}
	hasher, err := highwayhash.New64(hashKey[:])
	if err != nil {
		return "", fmt.Errorf("failed to create hasher: %w", err)
	}
	if _, err := io.Copy(hasher, bufio.NewReader(exeFile)); err != nil {
		return "", fmt.Errorf("failed to hash executable: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
