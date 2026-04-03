package systemcontract

import (
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"
)

func TestSetupEffectiveStatusHTTPContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "025-powerx-docker-systemd", "contracts", "http-openapi.yaml")
	loader := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := loader.LoadFromFile(specPath)
	require.NoError(t, err, "load setup openapi spec")

	schema := doc.Components.Schemas["SetupStatus"]
	require.NotNil(t, schema, "SetupStatus schema missing")
	require.Contains(t, schema.Value.Properties, "desired_ports", "SetupStatus.desired_ports missing")
	require.Contains(t, schema.Value.Properties, "effective_ports", "SetupStatus.effective_ports missing")
	require.Contains(t, schema.Value.Properties, "restart_required", "SetupStatus.restart_required missing")
	require.Contains(t, schema.Value.Properties, "config_source", "SetupStatus.config_source missing")
}
