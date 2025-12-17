package integrationgatewaycontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIntegrationGatewayProtoContract performs a lightweight contract validation
// against specs/007.../contracts/integration-gateway.proto to ensure the
// capability registry RPC surface remains intact before runtime implementation arrives.
func TestIntegrationGatewayProtoContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "007-integration-gateway-and-mcp", "contracts", "integration-gateway.proto")
	content, err := os.ReadFile(specPath)
	require.NoError(t, err, "failed to read gRPC contract proto: %s", specPath)

	src := string(content)
	require.True(t, strings.Contains(src, "service IntegrationGatewayService"), "service definition missing")

	rpcs := []string{
		"rpc ListCapabilities",
		"rpc GetCapability",
		"rpc InvokeCapability",
		"rpc ListWorkflowTemplates",
		"rpc StreamInvocation",
		"rpc ListCapabilitySyncJobs",
	}
	for _, rpc := range rpcs {
		require.Containsf(t, src, rpc, "integration-gateway.proto missing %s", rpc)
	}

	messages := []string{
		"message ListCapabilitiesRequest",
		"message Capability",
		"message ProtocolBinding",
		"message WorkflowTemplate",
		"message ListCapabilitiesResponse",
		"message GetCapabilityRequest",
		"message InvokeCapabilityRequest",
		"message InvokeCapabilityResponse",
		"message ListWorkflowTemplatesRequest",
		"message ListCapabilitySyncJobsRequest",
		"message CapabilitySyncJob",
	}
	for _, msg := range messages {
		require.Containsf(t, src, msg, "integration-gateway.proto missing %s", msg)
	}
}
