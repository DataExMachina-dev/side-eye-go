// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package machinapb

import (
	context "context"
	chunkpb "github.com/DataExMachina-dev/side-eye-go/internal/chunkpb"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// MachinaClient is the client API for Machina service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type MachinaClient interface {
	// WatchProcesses returns a stream of processes which match the given
	// predicates.
	WatchProcesses(ctx context.Context, in *WatchProcessesRequest, opts ...grpc.CallOption) (Machina_WatchProcessesClient, error)
	// GetExecutable returns a stream of chunks which make up the executable.
	GetExecutable(ctx context.Context, in *GetExecutableRequest, opts ...grpc.CallOption) (Machina_GetExecutableClient, error)
	// Snapshot sets up and performs a snapshot of the given process.
	// The protocol is designed to allow for many snapshots to be taken close in
	// time across many processes and machinas. In order to make this possible,
	// the protocol has two phases of execution: Setup and Snapshot.
	//
	// Setup: The client sends a Setup message to the server. The server does not
	// respond with headers until it has finished setting up the snapshot. Any
	// additional messages sent by the client at this point may result in an error
	// until the headers have been received by the client. At this point, the
	// server may download the needed snapshot artifacts if it does not already
	// have them using the key in the setup request.
	//
	// Snapshot: Once the headers have been received by the client, the client may
	// send a Snapshot message to the server. The server will respond with a
	// stream that has a single SnapshotResponse message.
	//
	// The protocol may be extended in the future to allow for multiple snapshots.
	Snapshot(ctx context.Context, opts ...grpc.CallOption) (Machina_SnapshotClient, error)
	// GetMetadata returns metadata about the machina.
	//
	// The response is streaming so that ex can detect disconnections from
	// the machina.
	MachinaInfo(ctx context.Context, in *MachinaInfoRequest, opts ...grpc.CallOption) (Machina_MachinaInfoClient, error)
}

type machinaClient struct {
	cc grpc.ClientConnInterface
}

func NewMachinaClient(cc grpc.ClientConnInterface) MachinaClient {
	return &machinaClient{cc}
}

func (c *machinaClient) WatchProcesses(ctx context.Context, in *WatchProcessesRequest, opts ...grpc.CallOption) (Machina_WatchProcessesClient, error) {
	stream, err := c.cc.NewStream(ctx, &Machina_ServiceDesc.Streams[0], "/machina.Machina/WatchProcesses", opts...)
	if err != nil {
		return nil, err
	}
	x := &machinaWatchProcessesClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Machina_WatchProcessesClient interface {
	Recv() (*Update, error)
	grpc.ClientStream
}

type machinaWatchProcessesClient struct {
	grpc.ClientStream
}

func (x *machinaWatchProcessesClient) Recv() (*Update, error) {
	m := new(Update)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *machinaClient) GetExecutable(ctx context.Context, in *GetExecutableRequest, opts ...grpc.CallOption) (Machina_GetExecutableClient, error) {
	stream, err := c.cc.NewStream(ctx, &Machina_ServiceDesc.Streams[1], "/machina.Machina/GetExecutable", opts...)
	if err != nil {
		return nil, err
	}
	x := &machinaGetExecutableClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Machina_GetExecutableClient interface {
	Recv() (*chunkpb.Chunk, error)
	grpc.ClientStream
}

type machinaGetExecutableClient struct {
	grpc.ClientStream
}

func (x *machinaGetExecutableClient) Recv() (*chunkpb.Chunk, error) {
	m := new(chunkpb.Chunk)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *machinaClient) Snapshot(ctx context.Context, opts ...grpc.CallOption) (Machina_SnapshotClient, error) {
	stream, err := c.cc.NewStream(ctx, &Machina_ServiceDesc.Streams[2], "/machina.Machina/Snapshot", opts...)
	if err != nil {
		return nil, err
	}
	x := &machinaSnapshotClient{stream}
	return x, nil
}

type Machina_SnapshotClient interface {
	Send(*SnapshotRequest) error
	Recv() (*SnapshotResponse, error)
	grpc.ClientStream
}

type machinaSnapshotClient struct {
	grpc.ClientStream
}

func (x *machinaSnapshotClient) Send(m *SnapshotRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *machinaSnapshotClient) Recv() (*SnapshotResponse, error) {
	m := new(SnapshotResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *machinaClient) MachinaInfo(ctx context.Context, in *MachinaInfoRequest, opts ...grpc.CallOption) (Machina_MachinaInfoClient, error) {
	stream, err := c.cc.NewStream(ctx, &Machina_ServiceDesc.Streams[3], "/machina.Machina/MachinaInfo", opts...)
	if err != nil {
		return nil, err
	}
	x := &machinaMachinaInfoClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Machina_MachinaInfoClient interface {
	Recv() (*MachinaInfoResponse, error)
	grpc.ClientStream
}

type machinaMachinaInfoClient struct {
	grpc.ClientStream
}

func (x *machinaMachinaInfoClient) Recv() (*MachinaInfoResponse, error) {
	m := new(MachinaInfoResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// MachinaServer is the server API for Machina service.
// All implementations must embed UnimplementedMachinaServer
// for forward compatibility
type MachinaServer interface {
	// WatchProcesses returns a stream of processes which match the given
	// predicates.
	WatchProcesses(*WatchProcessesRequest, Machina_WatchProcessesServer) error
	// GetExecutable returns a stream of chunks which make up the executable.
	GetExecutable(*GetExecutableRequest, Machina_GetExecutableServer) error
	// Snapshot sets up and performs a snapshot of the given process.
	// The protocol is designed to allow for many snapshots to be taken close in
	// time across many processes and machinas. In order to make this possible,
	// the protocol has two phases of execution: Setup and Snapshot.
	//
	// Setup: The client sends a Setup message to the server. The server does not
	// respond with headers until it has finished setting up the snapshot. Any
	// additional messages sent by the client at this point may result in an error
	// until the headers have been received by the client. At this point, the
	// server may download the needed snapshot artifacts if it does not already
	// have them using the key in the setup request.
	//
	// Snapshot: Once the headers have been received by the client, the client may
	// send a Snapshot message to the server. The server will respond with a
	// stream that has a single SnapshotResponse message.
	//
	// The protocol may be extended in the future to allow for multiple snapshots.
	Snapshot(Machina_SnapshotServer) error
	// GetMetadata returns metadata about the machina.
	//
	// The response is streaming so that ex can detect disconnections from
	// the machina.
	MachinaInfo(*MachinaInfoRequest, Machina_MachinaInfoServer) error
	mustEmbedUnimplementedMachinaServer()
}

// UnimplementedMachinaServer must be embedded to have forward compatible implementations.
type UnimplementedMachinaServer struct {
}

func (UnimplementedMachinaServer) WatchProcesses(*WatchProcessesRequest, Machina_WatchProcessesServer) error {
	return status.Errorf(codes.Unimplemented, "method WatchProcesses not implemented")
}
func (UnimplementedMachinaServer) GetExecutable(*GetExecutableRequest, Machina_GetExecutableServer) error {
	return status.Errorf(codes.Unimplemented, "method GetExecutable not implemented")
}
func (UnimplementedMachinaServer) Snapshot(Machina_SnapshotServer) error {
	return status.Errorf(codes.Unimplemented, "method Snapshot not implemented")
}
func (UnimplementedMachinaServer) MachinaInfo(*MachinaInfoRequest, Machina_MachinaInfoServer) error {
	return status.Errorf(codes.Unimplemented, "method MachinaInfo not implemented")
}
func (UnimplementedMachinaServer) mustEmbedUnimplementedMachinaServer() {}

// UnsafeMachinaServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to MachinaServer will
// result in compilation errors.
type UnsafeMachinaServer interface {
	mustEmbedUnimplementedMachinaServer()
}

func RegisterMachinaServer(s grpc.ServiceRegistrar, srv MachinaServer) {
	s.RegisterService(&Machina_ServiceDesc, srv)
}

func _Machina_WatchProcesses_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(WatchProcessesRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(MachinaServer).WatchProcesses(m, &machinaWatchProcessesServer{stream})
}

type Machina_WatchProcessesServer interface {
	Send(*Update) error
	grpc.ServerStream
}

type machinaWatchProcessesServer struct {
	grpc.ServerStream
}

func (x *machinaWatchProcessesServer) Send(m *Update) error {
	return x.ServerStream.SendMsg(m)
}

func _Machina_GetExecutable_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(GetExecutableRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(MachinaServer).GetExecutable(m, &machinaGetExecutableServer{stream})
}

type Machina_GetExecutableServer interface {
	Send(*chunkpb.Chunk) error
	grpc.ServerStream
}

type machinaGetExecutableServer struct {
	grpc.ServerStream
}

func (x *machinaGetExecutableServer) Send(m *chunkpb.Chunk) error {
	return x.ServerStream.SendMsg(m)
}

func _Machina_Snapshot_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(MachinaServer).Snapshot(&machinaSnapshotServer{stream})
}

type Machina_SnapshotServer interface {
	Send(*SnapshotResponse) error
	Recv() (*SnapshotRequest, error)
	grpc.ServerStream
}

type machinaSnapshotServer struct {
	grpc.ServerStream
}

func (x *machinaSnapshotServer) Send(m *SnapshotResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *machinaSnapshotServer) Recv() (*SnapshotRequest, error) {
	m := new(SnapshotRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _Machina_MachinaInfo_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(MachinaInfoRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(MachinaServer).MachinaInfo(m, &machinaMachinaInfoServer{stream})
}

type Machina_MachinaInfoServer interface {
	Send(*MachinaInfoResponse) error
	grpc.ServerStream
}

type machinaMachinaInfoServer struct {
	grpc.ServerStream
}

func (x *machinaMachinaInfoServer) Send(m *MachinaInfoResponse) error {
	return x.ServerStream.SendMsg(m)
}

// Machina_ServiceDesc is the grpc.ServiceDesc for Machina service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Machina_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "machina.Machina",
	HandlerType: (*MachinaServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "WatchProcesses",
			Handler:       _Machina_WatchProcesses_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "GetExecutable",
			Handler:       _Machina_GetExecutable_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "Snapshot",
			Handler:       _Machina_Snapshot_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "MachinaInfo",
			Handler:       _Machina_MachinaInfo_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "machina.proto",
}