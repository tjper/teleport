// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package v1

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// JobWorkerServiceClient is the client API for JobWorkerService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type JobWorkerServiceClient interface {
	Start(ctx context.Context, in *StartRequest, opts ...grpc.CallOption) (*StartResponse, error)
	Stop(ctx context.Context, in *StopRequest, opts ...grpc.CallOption) (*StopResponse, error)
	Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error)
	Output(ctx context.Context, in *OutputRequest, opts ...grpc.CallOption) (JobWorkerService_OutputClient, error)
}

type jobWorkerServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewJobWorkerServiceClient(cc grpc.ClientConnInterface) JobWorkerServiceClient {
	return &jobWorkerServiceClient{cc}
}

func (c *jobWorkerServiceClient) Start(ctx context.Context, in *StartRequest, opts ...grpc.CallOption) (*StartResponse, error) {
	out := new(StartResponse)
	err := c.cc.Invoke(ctx, "/jobworker.v1.JobWorkerService/Start", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jobWorkerServiceClient) Stop(ctx context.Context, in *StopRequest, opts ...grpc.CallOption) (*StopResponse, error) {
	out := new(StopResponse)
	err := c.cc.Invoke(ctx, "/jobworker.v1.JobWorkerService/Stop", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jobWorkerServiceClient) Status(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error) {
	out := new(StatusResponse)
	err := c.cc.Invoke(ctx, "/jobworker.v1.JobWorkerService/Status", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jobWorkerServiceClient) Output(ctx context.Context, in *OutputRequest, opts ...grpc.CallOption) (JobWorkerService_OutputClient, error) {
	stream, err := c.cc.NewStream(ctx, &JobWorkerService_ServiceDesc.Streams[0], "/jobworker.v1.JobWorkerService/Output", opts...)
	if err != nil {
		return nil, err
	}
	x := &jobWorkerServiceOutputClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type JobWorkerService_OutputClient interface {
	Recv() (*OutputResponse, error)
	grpc.ClientStream
}

type jobWorkerServiceOutputClient struct {
	grpc.ClientStream
}

func (x *jobWorkerServiceOutputClient) Recv() (*OutputResponse, error) {
	m := new(OutputResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// JobWorkerServiceServer is the server API for JobWorkerService service.
// All implementations should embed UnimplementedJobWorkerServiceServer
// for forward compatibility
type JobWorkerServiceServer interface {
	Start(context.Context, *StartRequest) (*StartResponse, error)
	Stop(context.Context, *StopRequest) (*StopResponse, error)
	Status(context.Context, *StatusRequest) (*StatusResponse, error)
	Output(*OutputRequest, JobWorkerService_OutputServer) error
}

// UnimplementedJobWorkerServiceServer should be embedded to have forward compatible implementations.
type UnimplementedJobWorkerServiceServer struct {
}

func (UnimplementedJobWorkerServiceServer) Start(context.Context, *StartRequest) (*StartResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Start not implemented")
}
func (UnimplementedJobWorkerServiceServer) Stop(context.Context, *StopRequest) (*StopResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Stop not implemented")
}
func (UnimplementedJobWorkerServiceServer) Status(context.Context, *StatusRequest) (*StatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Status not implemented")
}
func (UnimplementedJobWorkerServiceServer) Output(*OutputRequest, JobWorkerService_OutputServer) error {
	return status.Errorf(codes.Unimplemented, "method Output not implemented")
}

// UnsafeJobWorkerServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to JobWorkerServiceServer will
// result in compilation errors.
type UnsafeJobWorkerServiceServer interface {
	mustEmbedUnimplementedJobWorkerServiceServer()
}

func RegisterJobWorkerServiceServer(s grpc.ServiceRegistrar, srv JobWorkerServiceServer) {
	s.RegisterService(&JobWorkerService_ServiceDesc, srv)
}

func _JobWorkerService_Start_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StartRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JobWorkerServiceServer).Start(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/jobworker.v1.JobWorkerService/Start",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JobWorkerServiceServer).Start(ctx, req.(*StartRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JobWorkerService_Stop_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StopRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JobWorkerServiceServer).Stop(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/jobworker.v1.JobWorkerService/Stop",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JobWorkerServiceServer).Stop(ctx, req.(*StopRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JobWorkerService_Status_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JobWorkerServiceServer).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/jobworker.v1.JobWorkerService/Status",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JobWorkerServiceServer).Status(ctx, req.(*StatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JobWorkerService_Output_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(OutputRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(JobWorkerServiceServer).Output(m, &jobWorkerServiceOutputServer{stream})
}

type JobWorkerService_OutputServer interface {
	Send(*OutputResponse) error
	grpc.ServerStream
}

type jobWorkerServiceOutputServer struct {
	grpc.ServerStream
}

func (x *jobWorkerServiceOutputServer) Send(m *OutputResponse) error {
	return x.ServerStream.SendMsg(m)
}

// JobWorkerService_ServiceDesc is the grpc.ServiceDesc for JobWorkerService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var JobWorkerService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "jobworker.v1.JobWorkerService",
	HandlerType: (*JobWorkerServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Start",
			Handler:    _JobWorkerService_Start_Handler,
		},
		{
			MethodName: "Stop",
			Handler:    _JobWorkerService_Stop_Handler,
		},
		{
			MethodName: "Status",
			Handler:    _JobWorkerService_Status_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Output",
			Handler:       _JobWorkerService_Output_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "jobworker/v1/service_api.proto",
}
