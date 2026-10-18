package grpcserver

import (
	"context"
	"testing"

	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRecoveryRPCUsesExplicitAllowlistAndRejectsCommandStreams(t *testing.T) {
	for _, method := range []string{"/grpc.health.v1.Health/Check", stsv1.STSService_Exchange_FullMethodName, "/powerx.orders.v1.Orders/GetAndDelete", "/powerx.workflow.v1.Workflow/Invoke"} {
		called := false
		_, e := recoveryUnary(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: method}, func(context.Context, any) (any, error) { called = true; return nil, nil })
		allowed := method == "/grpc.health.v1.Health/Check" || method == stsv1.STSService_Exchange_FullMethodName
		require.Equal(t, allowed, called, method)
		if allowed {
			require.NoError(t, e)
		} else {
			require.Equal(t, codes.Unavailable, status.Code(e))
		}
	}
	called := false
	e := recoveryStream(nil, nil, &grpc.StreamServerInfo{FullMethod: "/powerx.workflow.v1.Workflow/Stream"}, func(any, grpc.ServerStream) error { called = true; return nil })
	require.False(t, called)
	require.Equal(t, codes.Unavailable, status.Code(e))
}
