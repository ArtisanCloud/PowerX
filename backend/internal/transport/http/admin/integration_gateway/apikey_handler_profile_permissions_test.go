package integration_gateway

import (
	"encoding/json"
	"testing"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelsiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
)

func TestProfilePermissionResponseExcludesUnresolvableAPIKeyGrants(t *testing.T) {
	apiKeyMeta, err := json.Marshal(map[string]any{
		"api_key": map[string]any{
			"scope":            "_scope.ai.catalog.models.read",
			"action":           "read",
			"resource_type":    "api",
			"resource_pattern": "ai-catalog-models",
			"effect":           "allow",
		},
	})
	if err != nil {
		t.Fatalf("marshal permission metadata: %v", err)
	}

	rows := []*modelsiam.Permission{
		{PowerModel: model.PowerModel{ID: 1}, Status: modelsiam.PermissionStatusActive, AllowAPIKey: true, Meta: apiKeyMeta},
		{PowerModel: model.PowerModel{ID: 2}, Status: modelsiam.PermissionStatusActive, AllowAPIKey: true},
		{PowerModel: model.PowerModel{ID: 3}, Status: modelsiam.PermissionStatusDeprecated, AllowAPIKey: true, Meta: apiKeyMeta},
		{PowerModel: model.PowerModel{ID: 4}, Status: modelsiam.PermissionStatusActive, AllowAPIKey: false, Meta: apiKeyMeta},
		nil,
	}

	got := profileAPIKeyGrantablePermissionIDs(rows)
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("grantable IDs = %v, want [1]", got)
	}
}
