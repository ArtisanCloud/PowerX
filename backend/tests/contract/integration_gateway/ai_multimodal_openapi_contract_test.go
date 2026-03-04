package integrationgatewaycontract

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// TestAIMultimodalOpenAPIContract validates the public Multimodal OpenAPI spec
// keeps required endpoints, error codes, and SSE examples.
func TestAIMultimodalOpenAPIContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "007-integration-gateway-and-mcp", "contracts", "ai-multimodal.http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "failed to load multimodal OpenAPI: %s", specPath)

	require.NotNil(t, doc.Paths, "OpenAPI must define paths")
	_, ok := doc.Components.Schemas["ErrorResponse"]
	require.True(t, ok, "ErrorResponse schema missing")

	assertOperation(t, doc, "/ai/llm/invoke", http.MethodPost, []string{"200", "400", "403", "429", "502"})
	assertOperation(t, doc, "/ai/llm/sessions", http.MethodPost, []string{"200", "403"})
	assertOperation(t, doc, "/ai/llm/sessions/{session_id}/messages", http.MethodPost, []string{"200", "403"})
	assertOperation(t, doc, "/ai/llm/sessions/{session_id}/stream", http.MethodGet, []string{"200", "403"})
	assertOperation(t, doc, "/ai/image/invoke", http.MethodPost, []string{"200", "400", "403", "429", "502"})
	assertOperation(t, doc, "/ai/video/invoke", http.MethodPost, []string{"200", "400", "403", "429", "502"})
	assertOperation(t, doc, "/ai/tts/invoke", http.MethodPost, []string{"200", "400", "403", "429", "502"})
	assertOperation(t, doc, "/ai/embedding/invoke", http.MethodPost, []string{"200", "400", "403", "429"})

	assertSSEExample(t, doc, "/ai/llm/sessions/{session_id}/stream", http.MethodGet)
}
