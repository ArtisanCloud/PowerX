package grpcserver

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// 恢复验证采用明确的只读 RPC 清单，不按 Get/List 名称猜测写入风险。
var recoveryReadMethods = map[string]bool{
	"/grpc.health.v1.Health/Check":            true,
	"/powerx.auth.sts.v1.STSService/Exchange": true,
}

func recoveryUnary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if !recoveryReadMethods[info.FullMethod] {
		return nil, status.Error(codes.Unavailable, "RECOVERY_READ_ONLY")
	}
	return handler(ctx, req)
}
func recoveryStream(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return status.Error(codes.Unavailable, "RECOVERY_READ_ONLY")
}
