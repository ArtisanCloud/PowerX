package authorization_baseline

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestPublishedSchemaIsValidAndHasExclusiveResultBranches(t *testing.T) {
	loader := openapi3.NewLoader()
	document, err := loader.LoadFromFile(filepath.Join("..", "..", "..", "..", "specs", "026-iam", "contracts", "authorization-baseline.openapi.yaml"))
	require.NoError(t, err)
	require.NoError(t, document.Validate(context.Background()))
	require.Len(t, document.Components.Schemas["BaselineResult"].Value.Properties["items"].Value.Items.Value.OneOf, 3)
	require.NotContains(t, document.Components.Schemas["BaselineError"].Value.Properties, "allowed")
}
