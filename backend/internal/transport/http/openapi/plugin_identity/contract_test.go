package plugin_identity

import (
	"context"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFormalRequestIdentityContract(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../../../../../../specs/018-plugin-request-identity/contracts/identity-context.openapi.yaml")
	require.NoError(t, err)
	require.NoError(t, doc.Validate(context.Background()))
}
