package integrationgatewaycontract

import (
	"net/http"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

// TestAgentOpenAPIContract validates the public Agent OpenAPI spec keeps the
// required endpoints, error codes, and SSE examples.
func TestAgentOpenAPIContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "007-integration-gateway-and-mcp", "contracts", "agent.http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "failed to load agent OpenAPI: %s", specPath)

	require.NotNil(t, doc.Paths, "OpenAPI must define paths")
	_, ok := doc.Components.Schemas["ErrorResponse"]
	require.True(t, ok, "ErrorResponse schema missing")

	assertOperation(t, doc, "/agents/invoke", http.MethodPost, []string{"200", "400", "403", "404", "429"})
	assertOperation(t, doc, "/agents/stream/sse", http.MethodGet, []string{"200", "403", "429"})
	assertOperation(t, doc, "/agents/stream/ws", http.MethodGet, []string{"101"})
	assertOperation(t, doc, "/agents/sessions", http.MethodPost, []string{"200", "403", "429"})
	assertOperation(t, doc, "/agents/sessions/{session_id}/messages", http.MethodGet, []string{"200", "403", "404", "429"})

	assertSSEExample(t, doc, "/agents/stream/sse", http.MethodGet)
}

func assertSSEExample(t testing.TB, doc *openapi3.T, path, method string) {
	t.Helper()
	item, ok := doc.Paths[path]
	require.True(t, ok, "path %s missing from OpenAPI", path)
	op := getOperation(item, method)
	require.NotNil(t, op, "path %s missing %s operation", path, method)
	respRef, ok := op.Responses["200"]
	require.True(t, ok, "operation %s %s missing response 200", method, path)
	require.NotNil(t, respRef.Value, "response 200 missing")
	media := respRef.Value.Content.Get("text/event-stream")
	require.NotNil(t, media, "response 200 missing text/event-stream content")
	require.True(t, len(media.Examples) > 0 || media.Example != nil, "SSE example missing for %s %s", method, path)
}
