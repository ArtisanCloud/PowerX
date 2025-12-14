//go:build ignore

package capabilitycontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capabilityhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const capabilityAdminTenantUUID = "5f85643a-4509-4a61-a0dd-f325c6a2a92a"

func TestCapabilityAdminHTTPContractFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deps := setupCapabilityDeps(t)

	engine := gin.New()
	group := engine.Group("/api/admin/capabilities")
	group.Use(requireCapabilityAuth(capabilityAdminTenantUUID))

	contractHandler := capabilityhttp.NewContractHandler(deps)
	group.POST("", contractHandler.CreateContract)
	group.GET("", contractHandler.ListContracts)
	group.GET(":capabilityKey/versions/:version", contractHandler.GetContract)

	capKey := "cap.crm.sync"
	version := "1.0.0"
	payload := map[string]any{
		"capability_key":      capKey,
		"version":             version,
		"provider_id":         "provider.crm",
		"display_name":        "CRM Sync",
		"description":         "syncs CRM contacts",
		"security_scope":      "tenant.admin",
		"tool_grant_required": true,
		"observability_config": map[string]any{
			"metrics": []string{"latency_ms"},
		},
		"io_schemas": []map[string]any{
			{
				"direction":        "input",
				"format":           "json",
				"schema_uri":       "https://schemas.powerx.dev/cap.crm.sync/input",
				"schema_hash":      "sha256:12345",
				"validation_rules": map[string]any{"required": true},
			},
		},
		"transport_preferences": []map[string]any{
			{
				"transport": "http",
				"mode":      "prefer",
			},
		},
		"transport_profiles": []map[string]any{
			{
				"transport":         "http",
				"mode":              "prefer",
				"timeout_ms":        5000,
				"streaming":         false,
				"retry":             map[string]any{"max": 3},
				"qos":               map[string]any{"priority": "normal"},
				"endpoint_selector": map[string]any{"region": "ap"},
			},
		},
		"error_taxonomy": []map[string]any{
			{
				"namespace": "system",
				"category":  "internal",
				"code":      "INTERNAL_ERROR",
				"severity":  "error",
				"stage":     "invoke",
			},
		},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	unauthorizedReq := httptest.NewRequest(http.MethodPost, "/api/admin/capabilities", bytes.NewReader(body))
	unauthorizedReq.Header.Set("Content-Type", "application/json")
	unauthorizedResp := httptest.NewRecorder()
	engine.ServeHTTP(unauthorizedResp, unauthorizedReq)
	require.Equal(t, http.StatusUnauthorized, unauthorizedResp.Code)

	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/capabilities", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createResp := serveCapabilityAdminRequest(t, engine, createReq, capabilityAdminTenantUUID)
	require.Equal(t, http.StatusOK, createResp.Code)

	var createBody struct {
		Code int `json:"code"`
		Data struct {
			CapabilityKey string `json:"capability_key"`
			Version       string `json:"version"`
			TenantUUID    string `json:"tenant_uuid"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &createBody))
	require.Equal(t, capKey, createBody.Data.CapabilityKey)
	require.Equal(t, version, createBody.Data.Version)
	require.Equal(t, capabilityAdminTenantUUID, createBody.Data.TenantUUID)

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/capabilities?capability_key="+capKey, nil)
	listResp := serveCapabilityAdminRequest(t, engine, listReq, capabilityAdminTenantUUID)
	require.Equal(t, http.StatusOK, listResp.Code)

	missingReq := httptest.NewRequest(http.MethodGet, "/api/admin/capabilities", nil)
	missingReq.Header.Set("Authorization", "Bearer admin")
	missingResp := httptest.NewRecorder()
	engine.ServeHTTP(missingResp, missingReq)
	require.Equal(t, http.StatusUnauthorized, missingResp.Code)

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/admin/capabilities/%s/versions/%s", capKey, version), nil)
	getResp := serveCapabilityAdminRequest(t, engine, getReq, capabilityAdminTenantUUID)
	require.Equal(t, http.StatusOK, getResp.Code)
}

func setupCapabilityDeps(t *testing.T) *shared.Deps {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE :memory: AS public").Error)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&capmodel.CapabilityContract{},
		&capmodel.CapabilityIOSchema{},
		&capmodel.CapabilityErrorTaxonomy{},
		&capmodel.CapabilityContractErrorTaxonomy{},
		&capmodel.CapabilityTransportProfile{},
		&capmodel.CapabilityVersionPolicy{},
		&auditmodel.AuditEvent{},
	))

	auditSvc := auditsvc.NewService(auditsvc.ServiceOptions{
		DB: db,
		Config: auditsvc.AuditOptions{
			BatchSize: 1,
			BatchWait: time.Millisecond,
		},
	})
	t.Cleanup(func() { auditSvc.Close() })

	return &shared.Deps{
		DB:       db,
		AuditSvc: auditSvc,
	}
}
