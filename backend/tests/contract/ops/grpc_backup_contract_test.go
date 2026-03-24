package opscontract

import (
	"testing"

	platformopsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/platform_ops/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestBackupGRPCDescriptors(t *testing.T) {
	file := platformopsv1.File_powerx_platform_ops_v1_ops_admin_proto
	require.NotNil(t, file, "ops admin proto descriptor missing")

	svc := requireOpsServiceForBackup(t, file)
	assertUnaryMethodForBackup(t, svc, "ListBackupPolicies")
	assertUnaryMethodForBackup(t, svc, "UpsertBackupPolicy")
	assertUnaryMethodForBackup(t, svc, "TriggerBackupJob")
	assertUnaryMethodForBackup(t, svc, "ListBackupJobs")
	assertUnaryMethodForBackup(t, svc, "TriggerRestoreDrill")
}

func requireOpsServiceForBackup(t testing.TB, file protoreflect.FileDescriptor) protoreflect.ServiceDescriptor {
	t.Helper()
	svc := file.Services().ByName(protoreflect.Name("OpsAdminService"))
	require.NotNil(t, svc, "service OpsAdminService missing")
	return svc
}

func assertUnaryMethodForBackup(t testing.TB, svc protoreflect.ServiceDescriptor, methodName string) {
	t.Helper()
	m := svc.Methods().ByName(protoreflect.Name(methodName))
	require.NotNil(t, m, "method %s.%s missing", svc.FullName(), methodName)
	require.False(t, m.IsStreamingClient(), "method %s.%s should be unary client", svc.FullName(), methodName)
	require.False(t, m.IsStreamingServer(), "method %s.%s should be unary server", svc.FullName(), methodName)
}
