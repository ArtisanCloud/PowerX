package integrationgatewaycontract

import (
	"context"
	"encoding/json"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestAIMediaV2PublishedSchemaAndSharedFixtures(t *testing.T) {
	root := repoRootFromHere(t)
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(filepath.Join(root, "docs/contracts/ai-media-v2.openapi.yaml"))
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()))
	raw, err := os.ReadFile(filepath.Join(root, "docs/contracts/fixtures/ai-media-v2.json"))
	require.NoError(t, err)
	var fixture map[string]any
	require.NoError(t, json.Unmarshal(raw, &fixture))
	for field, schema := range map[string]string{"catalog_operation": "CatalogOperation", "catalog_result": "MediaCatalog", "invoke_operation": "MediaInvokeOperation", "task_operation": "TaskOperation", "cancel_operation": "TaskOperation", "task_result": "MediaTask", "parameter_error": "ErrorEnvelope"} {
		require.NoError(t, doc.Components.Schemas[schema].Value.VisitJSON(fixture[field]), field)
	}
	for _, path := range []string{"/ai/vlm/invoke", "/ai/image/invoke", "/ai/video/invoke", "/ai/tts/invoke", "/ai/media/tasks/{task_uuid}", "/ai/media/tasks/{task_uuid}/cancel", "/ai/media/tasks/{task_uuid}/events"} {
		require.NotNil(t, doc.Paths[path], path)
	}
}
