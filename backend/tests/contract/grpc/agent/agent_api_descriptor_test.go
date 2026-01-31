package agentgrpccontract

import (
	"testing"

	agentv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestAgentGRPCDescriptors ensures the Agent gRPC contracts expose expected services/methods.
func TestAgentGRPCDescriptors(t *testing.T) {
	apiFile := agentv1.File_powerx_agent_v1_agent_api_proto
	require.NotNil(t, apiFile, "agent_api proto descriptor missing")

	invokeSvc := requireService(t, apiFile, "AgentInvokeService")
	assertMethod(t, invokeSvc, "Invoke", false, false)

	sessionSvc := requireService(t, apiFile, "AgentSessionService")
	assertMethod(t, sessionSvc, "CreateSession", false, false)
	assertMethod(t, sessionSvc, "ListMessages", false, false)

	streamFile := agentv1.File_powerx_agent_v1_stream_proto
	require.NotNil(t, streamFile, "stream proto descriptor missing")
	streamSvc := requireService(t, streamFile, "AgentStreamService")
	assertMethod(t, streamSvc, "Stream", false, true)
	assertMethod(t, streamSvc, "Simulate", false, true)
}

func requireService(t testing.TB, file protoreflect.FileDescriptor, name string) protoreflect.ServiceDescriptor {
	t.Helper()
	svc := file.Services().ByName(protoreflect.Name(name))
	require.NotNil(t, svc, "service %s missing", name)
	return svc
}

func assertMethod(t testing.TB, svc protoreflect.ServiceDescriptor, name string, clientStream, serverStream bool) {
	t.Helper()
	method := svc.Methods().ByName(protoreflect.Name(name))
	require.NotNil(t, method, "method %s.%s missing", svc.FullName(), name)
	require.Equal(t, clientStream, method.IsStreamingClient(), "method %s.%s client streaming mismatch", svc.FullName(), name)
	require.Equal(t, serverStream, method.IsStreamingServer(), "method %s.%s server streaming mismatch", svc.FullName(), name)
}
