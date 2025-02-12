package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/DataExMachina-dev/side-eye-go/internal/boottime"
	"golang.org/x/sync/errgroup"
	"io"
	"net"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"time"

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
	machinapb.UnimplementedGoPprofServer
}

var _ machinapb.MachinaServer = (*Server)(nil)
var _ machinapb.GoPprofServer = (*Server)(nil)

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
	defer exeFile.Close()
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

func ipAddresses() []string {
	addrs, _ /* err */ := net.InterfaceAddrs()
	res := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
			if ip.IsLoopback() {
				continue
			}
		case *net.IPAddr:
			ip = v.IP
			if ip.IsLoopback() {
				continue
			}
		}
		res = append(res, ip.String())
	}
	return res
}

// MachinaInfo implements machinapb.MachinaServer.
func (s *Server) MachinaInfo(req *machinapb.MachinaInfoRequest, stream machinapb.Machina_MachinaInfoServer) error {
	ctx := stream.Context()
	hostname, _ /* ignore the error */ := os.Hostname()
	if err := stream.Send(&machinapb.MachinaInfoResponse{
		Fingerprint: s.agentFingerprint.String(),
		Version:     "0.1",
		TenantToken: s.tenantToken,
		Environment: s.environment,
		IsLibrary:   true,
		Hostname:    hostname,
		IpAddresses: ipAddresses(),
	}); err != nil {
		return fmt.Errorf("failed to send MachinaInfo: %w", err)
	}

	// Block forever (or until Ex disconnects). Ex expects this RPC to run for the
	// lifetime of the agent.
	<-ctx.Done()
	return ctx.Err()
}

func (s *Server) Events(stream machinapb.Machina_EventsServer) error {
	return fmt.Errorf("side-eye-go does not currently support events")
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
	snapshotProgram, err := s.fetcher.FetchSnapshotProgram(ctx, setupReq.Setup.Key)
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
	output, err := snapshot.Snapshot(snapshotProgram)
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
	defer exeFile.Close()
	hasher, err := highwayhash.New64(hashKey[:])
	if err != nil {
		return "", fmt.Errorf("failed to create hasher: %w", err)
	}
	if _, err := io.Copy(hasher, bufio.NewReader(exeFile)); err != nil {
		return "", fmt.Errorf("failed to hash executable: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// Capture implements machinapb.GoPprofServer interface.
func (s *Server) Capture(request *machinapb.CaptureRequest, server machinapb.GoPprof_CaptureServer) error {
	if request.ProcessFingerprint != s.processFingerprint {
		return status.Errorf(
			codes.PermissionDenied,
			"process fingerprint mismatch: got %s, want %s",
			request.ProcessFingerprint, s.processFingerprint,
		)
	}

	serializer := newSendSerializer(server)
	g, ctx := errgroup.WithContext(server.Context())
	explicitCpuProfile := request.Contents == machinapb.CaptureContents_EXECUTION_TRACE_AND_CPU_PROFILE
	if explicitCpuProfile {
		g.Go(func() error {
			return s.runCpuProfile(ctx, time.Second*time.Duration(request.Seconds), serializer)
		})
	}
	g.Go(func() error {
		// If the CPU profile was not explicitly requested, we still want to start a
		// CPU profile, in order for its data to be automatically included in the
		// execution trace.
		if !explicitCpuProfile {
			if err := pprof.StartCPUProfile(nilWriter{}); err != nil {
				return fmt.Errorf("failed to start CPU profile: %w", err)
			}
			defer pprof.StopCPUProfile()
		}
		return s.runExecutionTrace(ctx, time.Second*time.Duration(request.Seconds), serializer)
	})

	return g.Wait()
}

func (s *Server) runCpuProfile(ctx context.Context, duration time.Duration, serializer *sendSerializer) error {
	profileBuf := new(bytes.Buffer)
	// Note: nothing gets written to profileBuf until pprof.StopCPUProfile() is
	// called; the profile proto is not streamable. We'll read the whole buffer
	// after the profile is done.
	if err := pprof.StartCPUProfile(profileBuf); err != nil {
		return fmt.Errorf("failed to start CPU profile: %w", err)
	}
	msg := &machinapb.CaptureResponse{
		Message: &machinapb.CaptureResponse_CpuProfileStart_{
			CpuProfileStart: &machinapb.CaptureResponse_CpuProfileStart{
				DurationSeconds: uint32(duration / time.Second),
			},
		},
	}
	if err := serializer.Send(msg); err != nil {
		return fmt.Errorf("failed to send CPU profile start: %w", err)
	}

	stop := pprof.StopCPUProfile
	defer func() {
		if stop != nil {
			stop()
		}
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("CPU profiling canceled: %w", context.Cause(ctx))
	case <-time.After(duration):
	}
	pprof.StopCPUProfile()
	stop = nil // inhibit the deferred call

	// Send chunk by chunk from profileBuf. We send in chunk to not hit gRPC's
	// maximum message size.
	const chunkSize = 64 << 10
	data := profileBuf.Bytes()
	for len(data) > 0 {
		chunkLen := chunkSize
		if chunkLen > len(data) {
			chunkLen = len(data)
		}
		msg := &machinapb.CaptureResponse{
			Message: &machinapb.CaptureResponse_CpuProfileChunk{
				CpuProfileChunk: &chunkpb.Chunk{Data: data[:chunkLen]},
			},
		}
		if err := serializer.Send(msg); err != nil {
			return fmt.Errorf("failed to send CPU profile chunk: %w", err)
		}
		data = data[chunkLen:]
	}

	msg = &machinapb.CaptureResponse{
		Message: &machinapb.CaptureResponse_CpuProfileComplete_{
			CpuProfileComplete: &machinapb.CaptureResponse_CpuProfileComplete{},
		},
	}
	if err := serializer.Send(msg); err != nil {
		return fmt.Errorf("failed to send CPU profile start: %w", err)
	}

	return nil
}

func (s *Server) runExecutionTrace(ctx context.Context, duration time.Duration, serializer *sendSerializer) error {
	reader, writer := io.Pipe()
	if err := trace.Start(writer); err != nil {
		return fmt.Errorf("failed to start CPU profile: %w", err)
	}

	boot, err := boottime.BootTime()
	if err != nil {
		return fmt.Errorf("failed to get boot time: %w", err)
	}
	msg := &machinapb.CaptureResponse{
		Message: &machinapb.CaptureResponse_ExecutionTraceStart_{
			ExecutionTraceStart: &machinapb.CaptureResponse_ExecutionTraceStart{
				ApproximateBootTime: &timestamppb.Timestamp{Seconds: boot.Unix(), Nanos: int32(boot.UnixNano() % 1e9)},
				TraceStartMonotonic: uint64(time.Since(boot)),
				DurationSeconds:     uint32(duration / time.Second),
			},
		},
	}
	if err := serializer.Send(msg); err != nil {
		return err
	}

	stop := trace.Stop
	defer func() {
		if stop != nil {
			stop()
		}
	}()

	// Start the reader goroutine that reads execution trace data from the pipe.

	// A channel for the reader goroutine to signal that it failed. The channel is buffered
	// to allow the reader goroutine to
	errCh := make(chan error)
	// Wait for the reader goroutine to finish before returning. This ensures that
	// we don't leak the reader goroutine, and it also ensures that there's always
	// someone reading from errCh (so the channel can be unbuffered).
	defer func() {
		// Ignore the error. If we got here and the reader goroutine is still
		// running, then we're already returning an error.
		_ = <-errCh
	}()
	go func() {
		defer close(errCh)
		const chunkSize = 64 << 10
		buf := make([]byte, chunkSize)
		// Read and send chunk by chunk from profileBuf.
		for {
			// Read from the buffer until we have a full chunk.
			n := 0
			for n < chunkSize {
				var nn int
				nn, err := reader.Read(buf[n:])
				n += nn
				if err == io.EOF {
					break
				}
				if err != nil {
					errCh <- err
					return
				}
			}
			if n == 0 {
				break
			}

			msg := &machinapb.CaptureResponse{
				Message: &machinapb.CaptureResponse_ExecutionTraceChunk{
					ExecutionTraceChunk: &chunkpb.Chunk{Data: buf[:n]},
				},
			}
			if err := serializer.Send(msg); err != nil {
				errCh <- err
				return
			}
		}

		msg := &machinapb.CaptureResponse{
			Message: &machinapb.CaptureResponse_ExecutionTraceComplete_{},
		}
		if err := serializer.Send(msg); err != nil {
			errCh <- err
			return
		}
	}()

	select {
	case <-ctx.Done():
		err := fmt.Errorf("CPU profiling canceled: %w", context.Cause(ctx))
		// Signal the reader goroutine to terminate.
		if err := writer.CloseWithError(err); err != nil {
			panic(fmt.Errorf("unexpected error from writer.Close: %w", err))
		}
		return err
	case err := <-errCh:
		return err
	case <-time.After(duration):
		trace.Stop()
		stop = nil // inhibit the deferred call
		// Signal the reader goroutine that the trace is done.
		if err := writer.Close(); err != nil {
			panic(fmt.Errorf("unexpected error from writer.Close: %w", err))
		}
		// Wait for the reader goroutine to finish.
		return <-errCh
	}
}

// sendSerializer serializes write access to a gRPC stream.
type sendSerializer struct {
	mu struct {
		sync.Mutex
		server machinapb.GoPprof_CaptureServer
	}
}

func newSendSerializer(server machinapb.GoPprof_CaptureServer) *sendSerializer {
	res := &sendSerializer{}
	res.mu.server = server
	return res
}

func (s *sendSerializer) Send(msg *machinapb.CaptureResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.mu.server.Send(msg); err != nil {
		return fmt.Errorf("failed to stream response to client: %w", err)
	}
	return nil
}

type nilWriter struct{}

func (w nilWriter) Write(b []byte) (int, error) {
	return len(b), nil
}
