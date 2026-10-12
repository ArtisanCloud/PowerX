package runtime_host

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIMatchesRegisteredRoutes(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "..", "..", "specs", "contracts", "runtime-host.openapi.yaml")
	doc, e := openapi3.NewLoader().LoadFromFile(path)
	require.NoError(t, e)
	require.NoError(t, doc.Validate(context.Background()))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterTenantRoutes(router.Group("/api/v1"), &shared.Deps{})
	count := 0
	for path, item := range doc.Paths {
		for method, operation := range item.Operations() {
			count++
			require.NotNil(t, operation.Responses["200"])
			found := false
			for _, route := range router.Routes() {
				if route.Method == method && route.Path == strings.ReplaceAll(path, "{task_uuid}", ":task_uuid") {
					found = true
				}
			}
			require.True(t, found, method+" "+path)
		}
	}
	require.Equal(t, 6, count)
	require.Len(t, router.Routes(), count)
}
