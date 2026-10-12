package agent_session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestOpenAPIAndCapabilityBindingsMatchRegisteredRoutes(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..", "..")
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(root, "specs/007-integration-gateway-and-mcp/contracts/agent-session.http-openapi.yaml"))
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()))
	raw, err := os.ReadFile(filepath.Join(root, "backend/config/platform_capabilities/agent.yaml"))
	require.NoError(t, err)
	var config struct {
		Capabilities []struct {
			ID        string `yaml:"capability_id"`
			Protocols []struct {
				Endpoint string `yaml:"endpoint"`
				Method   string `yaml:"method"`
				Actor    string `yaml:"actor_context"`
				Scope    string `yaml:"resource_scope"`
				Auth     string `yaml:"auth_type"`
				Direct   bool   `yaml:"sts_direct"`
			} `yaml:"protocols"`
		} `yaml:"capabilities"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &config))
	bindings := map[string]string{}
	for _, capability := range config.Capabilities {
		for _, binding := range capability.Protocols {
			if !strings.HasPrefix(binding.Endpoint, "/api/v1/tenant/agent/sessions") {
				continue
			}
			require.True(t, binding.Direct)
			require.Equal(t, "service_actor", binding.Actor)
			require.Equal(t, "tenant_plugin_service_actor", binding.Scope)
			require.Equal(t, "sts", binding.Auth)
			bindings[binding.Method+" "+binding.Endpoint] = capability.ID
		}
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(nil).Register(router.Group("/api/v1"))
	for _, route := range router.Routes() {
		path := strings.ReplaceAll(strings.ReplaceAll(route.Path, ":session_uuid", "{session_uuid}"), ":invocation_uuid", "{invocation_uuid}")
		item := doc.Paths[path]
		require.NotNil(t, item, route.Path)
		op := item.GetOperation(route.Method)
		require.NotNil(t, op, route.Method+" "+path)
		require.NotEmpty(t, bindings[route.Method+" "+path], route.Method+" "+path)
	}
	require.Len(t, bindings, len(router.Routes()))
}
