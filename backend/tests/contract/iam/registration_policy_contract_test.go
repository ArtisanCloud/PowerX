package iamcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	saashttp "github.com/ArtisanCloud/PowerX/internal/transport/http/public/saas"
	modelbase "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRegistrationPolicyOpenAPIContract(t *testing.T) {
	specPath := filepath.Join(repoRootFromHere(t), "specs", "026-iam", "contracts", "http-openapi.yaml")
	raw, err := os.ReadFile(specPath)
	require.NoError(t, err)

	var spec map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &spec))
	paths := spec["paths"].(map[string]any)

	effectivePath := paths["/public/saas/registration-policy/effective"].(map[string]any)
	effectiveGet := effectivePath["get"].(map[string]any)
	require.Equal(t, "getEffectiveSaaSRegistrationPolicy", effectiveGet["operationId"])

	requestPath := paths["/public/saas/registration-requests"].(map[string]any)
	requestPost := requestPath["post"].(map[string]any)
	require.Equal(t, "submitSaaSRegistrationRequest", requestPost["operationId"])

	rootPolicyPath := paths["/admin/registration-policy"].(map[string]any)
	require.Contains(t, rootPolicyPath, "get")
	require.Contains(t, rootPolicyPath, "put")

	rootPolicyHistoryPath := paths["/admin/registration-policy/history"].(map[string]any)
	require.Contains(t, rootPolicyHistoryPath, "get")

	rootActivatePath := paths["/admin/registration-policy/activate"].(map[string]any)
	require.Contains(t, rootActivatePath, "post")

	inviteBatchPath := paths["/admin/registration-invite-batches"].(map[string]any)
	require.Contains(t, inviteBatchPath, "post")
	require.Contains(t, inviteBatchPath, "get")
	require.Contains(t, inviteBatchPath, "delete")

	approvePath := paths["/admin/registration-requests/{request_uuid}/approve"].(map[string]any)
	require.Contains(t, approvePath, "post")
	rejectPath := paths["/admin/registration-requests/{request_uuid}/reject"].(map[string]any)
	require.Contains(t, rejectPath, "post")

	components := spec["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	effective := schemas["RegistrationPolicyEffective"].(map[string]any)
	mode := effective["properties"].(map[string]any)["mode"].(map[string]any)
	require.ElementsMatch(t, []any{"closed", "open", "invite_only", "waitlist", "approval_required", "allowlist", "progressive_rollout"}, mode["enum"].([]any))

	signup := schemas["SaaSSignupRequest"].(map[string]any)
	props := signup["properties"].(map[string]any)
	require.Contains(t, props, "invite_code")
	require.Contains(t, props, "channel")
	require.Contains(t, props, "campaign")
}

func TestSaaSSignupRejectsClosedRegistrationPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupRegistrationPolicyContractDB(t)
	now := time.Now().UTC()
	require.NoError(t, db.Create(&modeliam.RegistrationPolicy{
		PowerUUIDModel:       modelbase.PowerUUIDModel{UUID: uuid.New()},
		Version:              1,
		Mode:                 modeliam.RegistrationPolicyModeClosed,
		Status:               modeliam.RegistrationPolicyStatusActive,
		RequiresVerification: true,
		Rules:                datatypes.JSON([]byte(`[]`)),
		ActivatedAt:          &now,
		CreatedByUserUUID:    uuid.NewString(),
		UpdatedByUserUUID:    uuid.NewString(),
	}).Error)

	router := gin.New()
	saashttp.RegisterAPIRoutes(router.Group("/api/v1"), &shared.Deps{
		DB: db,
		AuthUser: authsvc.NewAuthService(db, authsvc.AuthOptions{
			JWTSecret: []byte("test-secret"),
			Issuer:    "powerx-test",
			Audience:  "powerx-test",
			AccessTTL: time.Hour,
		}),
	})

	body, err := json.Marshal(map[string]any{
		"tenant_name":        "Closed Tenant",
		"owner_email":        "owner@example.com",
		"owner_password":     "secret123",
		"owner_display_name": "Owner",
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/public/saas/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)
	require.Equal(t, http.StatusLocked, resp.Code)
	require.Contains(t, resp.Body.String(), "registration_closed")
}

func setupRegistrationPolicyContractDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared&_loc=UTC", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	prevSchema := modelbase.PowerXSchema
	modelbase.PowerXSchema = "main"
	t.Cleanup(func() { modelbase.PowerXSchema = prevSchema })
	require.NoError(t, db.AutoMigrate(&modeliam.RegistrationPolicy{}, &modeltenant.Tenant{}))
	return db
}
