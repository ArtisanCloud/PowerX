//go:build ignore

package agentmodelhubcontract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentmodelhubhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent_model_hub"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const routingTenantUUID = "tenant-routing-demo"

func TestRoutingHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newRoutingTestDB(t)

	engine := gin.New()
	public := engine.Group("/api")
	protected := engine.Group("/api")
	protected.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})

	deps := &shared.Deps{DB: db}
	agentmodelhubhttp.RegisterAPIRoutes(public, protected, deps)

	// Create routing policy
	policyPayload := map[string]any{
		"env": "default",
		"rules": []map[string]any{
			{
				"taskPattern": "chat",
				"candidates": []map[string]any{
					{"providerId": "provider-primary", "weight": 1},
					{"providerId": "provider-backup", "weight": 0.5},
				},
			},
		},
		"fallbackChain": []string{"provider-backup"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/internal/model-routing/policies", bytes.NewReader(mustJSON(t, policyPayload)))
	req.Header.Set("Content-Type", "application/json")
	rr := serveAgentModelHubRequest(t, engine, req, routingTenantUUID)
	require.Equal(t, http.StatusAccepted, rr.Code)

	// Promote policy via status endpoint
	statusPayload := map[string]any{
		"targetStatus": "active",
	}
	req = httptest.NewRequest(http.MethodPost, "/api/internal/model-routing/policies/status", bytes.NewReader(mustJSON(t, statusPayload)))
	req.Header.Set("Content-Type", "application/json")
	rr = serveAgentModelHubRequest(t, engine, req, routingTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code)

	// Request routing decision
	routePayload := map[string]any{
		"env": "default",
		"taskContext": map[string]any{
			"taskType": "chat",
		},
	}
	req = httptest.NewRequest(http.MethodPost, "/api/internal/model-routing/route", bytes.NewReader(mustJSON(t, routePayload)))
	req.Header.Set("Content-Type", "application/json")
	rr = serveAgentModelHubRequest(t, engine, req, routingTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code)

	var decision struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &decision))
	result := decision.Data
	require.Equal(t, "provider-primary", result["primaryProviderId"])

	// Trigger rollback
	rollbackPayload := map[string]any{
		"env":           "default",
		"targetVersion": 1,
	}
	req = httptest.NewRequest(http.MethodPost, "/api/internal/model-routing/rollback", bytes.NewReader(mustJSON(t, rollbackPayload)))
	req.Header.Set("Content-Type", "application/json")
	rr = serveAgentModelHubRequest(t, engine, req, routingTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code)

	// Toggle safe-mode
	safeModePayload := map[string]any{
		"enabled":    true,
		"ttlSeconds": 60,
	}
	req = httptest.NewRequest(http.MethodPost, "/api/internal/model-routing/safe-mode", bytes.NewReader(mustJSON(t, safeModePayload)))
	req.Header.Set("Content-Type", "application/json")
	rr = serveAgentModelHubRequest(t, engine, req, routingTenantUUID)
	require.Equal(t, http.StatusOK, rr.Code)
}

func newRoutingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	coremodel.PowerXSchema = ""
	require.NoError(t, db.AutoMigrate(&model.RoutingPolicy{}))
	return db
}

func mustJSON(t *testing.T, payload any) []byte {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return b
}
