package knowledge_content

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestFormalContentOverviewOpenAPI(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../../../../specs/011-knowledge-space/contracts/content-overview.openapi.yaml")
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()))
}
