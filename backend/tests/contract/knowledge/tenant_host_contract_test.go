package knowledgecontract

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestTenantKnowledgeHostHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "011-knowledge-space", "contracts", "http-openapi.yaml")
	doc, err := (&openapi3.Loader{IsExternalRefsAllowed: true}).LoadFromFile(specPath)
	require.NoError(t, err)

	for _, path := range []string{
		"/tenant/knowledge/spaces",
		"/tenant/knowledge/search",
		"/tenant/knowledge/spaces/{space_uuid}/documents",
		"/tenant/knowledge/spaces/{space_uuid}/documents/{document_uuid}",
		"/tenant/knowledge/spaces/{space_uuid}/indexes:rebuild",
		"/tenant/knowledge/index-jobs/{job_uuid}",
	} {
		require.NotNilf(t, doc.Paths.Find(path), "missing %s", path)
	}

	search := doc.Paths.Find("/tenant/knowledge/search").Post
	require.NotNil(t, search)
	request := search.RequestBody.Value.Content.Get("application/json").Schema.Value
	require.Contains(t, request.Required, "query")
	require.NotNil(t, request.Properties["space_uuids"])
	for _, status := range []string{"400", "401", "403", "404", "503"} {
		_, ok := search.Responses[status]
		require.Truef(t, ok, "search missing %s", status)
	}

	write := doc.Paths.Find("/tenant/knowledge/spaces/{space_uuid}/documents").Post
	require.NotNil(t, write)
	writeRequest := write.RequestBody.Value.Content.Get("application/json").Schema.Value
	require.ElementsMatch(t, []string{"title", "uri", "content", "content_type", "checksum", "version"}, writeRequest.Required)
	_, accepted := write.Responses["202"]
	require.True(t, accepted)
	_, conflict := write.Responses["409"]
	require.True(t, conflict)

	job := doc.Components.Schemas["TenantKnowledgeIndexJob"].Value
	require.NotNil(t, job.Properties["job_uuid"])
	require.NotNil(t, job.Properties["status"])
	require.ElementsMatch(t, []any{"queued", "running", "succeeded", "failed"}, job.Properties["status"].Value.Enum)
	errorSchema := doc.Components.Schemas["KnowledgeError"].Value
	require.NotNil(t, errorSchema.Properties["error_code"])
	require.NotNil(t, errorSchema.Properties["reason_code"])
}

func repoRootFromHere(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve current file path")
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
