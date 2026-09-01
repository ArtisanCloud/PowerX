package apikeypermissions

import (
	"testing"

	modelsiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/stretchr/testify/require"
)

func TestMergePermissionTriples_PlatformRowOverridesTemplateRow(t *testing.T) {
	rows := mergePermissionTriples([]modelsiam.Permission{
		{Module: "integration_gateway", Resource: "agent", Action: "invoke", Description: "template", Source: "template"},
		{Module: "integration_gateway", Resource: "agent", Action: "invoke", Description: "platform", Source: "platform_capability"},
		{Module: "integration_gateway", Resource: "agent", Action: "stream", Description: "different"},
	})
	require.Len(t, rows, 2)
	require.Equal(t, "platform", rows[0].Description)
	require.Equal(t, "platform_capability", rows[0].Source)
	require.Equal(t, "stream", rows[1].Action)
}
