package opscontract

import (
	"testing"

	platformopsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/platform_ops/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestDeployGRPCDescriptors(t *testing.T) {
	file := platformopsv1.File_powerx_platform_ops_v1_ops_admin_proto
	require.NotNil(t, file, "ops admin proto descriptor missing")

	svc := requireService(t, file, "OpsAdminService")
	assertMethod(t, svc, "ListDeployReleases")
	assertMethod(t, svc, "TriggerDeployRelease")
	assertMethod(t, svc, "TriggerDeployRollback")
	assertMethod(t, svc, "GetDeployHealth")
}

func requireService(t testing.TB, file protoreflect.FileDescriptor, name string) protoreflect.ServiceDescriptor {
	t.Helper()
	svc := file.Services().ByName(protoreflect.Name(name))
	require.NotNil(t, svc, "service %s missing", name)
	return svc
}

func assertMethod(t testing.TB, svc protoreflect.ServiceDescriptor, name string) {
	t.Helper()
	method := svc.Methods().ByName(protoreflect.Name(name))
	require.NotNil(t, method, "method %s.%s missing", svc.FullName(), name)
	require.False(t, method.IsStreamingClient(), "method %s.%s should be unary client", svc.FullName(), name)
	require.False(t, method.IsStreamingServer(), "method %s.%s should be unary server", svc.FullName(), name)
}
