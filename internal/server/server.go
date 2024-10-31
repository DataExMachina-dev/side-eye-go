package server

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
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
	// The ID that this process identifies as to the Side-Eye service, as it
	// connects as an agent.
	agentFingerprint uuid.UUID
	// processFingerprint is the process ID that will be reported to the Side-Eye
	// service.
	processFingerprint string

	tenantToken string
	environment string
	// The name of the program to be reported for the current process.
	programName string
	// ephemeralProcess is set if this process should not be visible in the
	// Side-Eye UI.
	ephemeralProcess bool
	fetcher          SnapshotFetcher

	hash binaryHashOnce

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
	agentFingerprint uuid.UUID,
	processFingerprint string,
	tenantToken string,
	environment string,
	programName string,
	fetcher SnapshotFetcher,
	ephemeralProcess bool,
) *Server {
	return &Server{
		agentFingerprint:   agentFingerprint,
		processFingerprint: processFingerprint,
		tenantToken:        tenantToken,
		environment:        environment,
		programName:        programName,
		fetcher:            fetcher,
		ephemeralProcess:   ephemeralProcess,
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
		Fingerprint: s.agentFingerprint.String(),
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
//
// The request proto is ignored. The current process is reported
// unconditionally, with the configured program name. Other processes are not
// reported.
func (s *Server) WatchProcesses(req *machinapb.WatchProcessesRequest, watchServer machinapb.Machina_WatchProcessesServer) error {
	ctx := watchServer.Context()
	hash, err := s.getBinaryHash()
	if err != nil {
		return fmt.Errorf("failed to get binary hash: %w", err)
	}
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
		Fingerprint: s.processFingerprint,
		Environment: s.environment,
		Program:     s.programName,
		Labels:      []*machinapb.LabelValue{{Label: "side-eye-go"}},
		Ephemeral:   s.ephemeralProcess,
	}

	if err := watchServer.Send(&machinapb.Update{
		Added: []*machinapb.Process{process},
	}); err != nil {
		return fmt.Errorf("failed to send Update: %w", err)
	}
	<-ctx.Done()
	return ctx.Err()
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
