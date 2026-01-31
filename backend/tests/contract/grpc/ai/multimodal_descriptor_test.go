package aigrpccontract

import (
	"testing"

	aiv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/ai/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestMultimodalGRPCDescriptors ensures the Multimodal gRPC contracts expose expected services/methods.
func TestMultimodalGRPCDescriptors(t *testing.T) {
	file := aiv1.File_powerx_ai_v1_multimodal_proto
	require.NotNil(t, file, "multimodal proto descriptor missing")

	mmSvc := requireService(t, file, "MultimodalService")
	assertMethod(t, mmSvc, "Invoke", false, false)
	assertMethod(t, mmSvc, "Stream", false, true)

	sessionSvc := requireService(t, file, "MultimodalSessionService")
	assertMethod(t, sessionSvc, "CreateSession", false, false)
	assertMethod(t, sessionSvc, "AppendMessage", false, false)

	embedSvc := requireService(t, file, "EmbeddingService")
	assertMethod(t, embedSvc, "Embed", false, false)
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
