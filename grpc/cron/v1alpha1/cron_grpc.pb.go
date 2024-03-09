//
//Copyright (c) 2024 Diagrid Inc.
//Licensed under the MIT License.

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.3.0
// - protoc             v4.24.4
// source: proto/v1alpha1/cron.proto

package cron

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

const (
	Cron_MyCoolCron_FullMethodName = "/proto.cron.v1alpha1.Cron/MyCoolCron"
)

// CronClient is the client API for Cron service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type CronClient interface {
	MyCoolCron(ctx context.Context, in *MyCoolCronRequest, opts ...grpc.CallOption) (*MyCoolCronResponse, error)
}

type cronClient struct {
	cc grpc.ClientConnInterface
}

func NewCronClient(cc grpc.ClientConnInterface) CronClient {
	return &cronClient{cc}
}

func (c *cronClient) MyCoolCron(ctx context.Context, in *MyCoolCronRequest, opts ...grpc.CallOption) (*MyCoolCronResponse, error) {
	out := new(MyCoolCronResponse)
	err := c.cc.Invoke(ctx, Cron_MyCoolCron_FullMethodName, in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CronServer is the server API for Cron service.
// All implementations must embed UnimplementedCronServer
// for forward compatibility
type CronServer interface {
	MyCoolCron(context.Context, *MyCoolCronRequest) (*MyCoolCronResponse, error)
	mustEmbedUnimplementedCronServer()
}

// UnimplementedCronServer must be embedded to have forward compatible implementations.
type UnimplementedCronServer struct {
}

func (UnimplementedCronServer) MyCoolCron(context.Context, *MyCoolCronRequest) (*MyCoolCronResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method MyCoolCron not implemented")
}
func (UnimplementedCronServer) mustEmbedUnimplementedCronServer() {}

// UnsafeCronServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to CronServer will
// result in compilation errors.
type UnsafeCronServer interface {
	mustEmbedUnimplementedCronServer()
}

func RegisterCronServer(s grpc.ServiceRegistrar, srv CronServer) {
	s.RegisterService(&Cron_ServiceDesc, srv)
}

func _Cron_MyCoolCron_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MyCoolCronRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(CronServer).MyCoolCron(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: Cron_MyCoolCron_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(CronServer).MyCoolCron(ctx, req.(*MyCoolCronRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Cron_ServiceDesc is the grpc.ServiceDesc for Cron service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Cron_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "proto.cron.v1alpha1.Cron",
	HandlerType: (*CronServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "MyCoolCron",
			Handler:    _Cron_MyCoolCron_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "proto/v1alpha1/cron.proto",
}