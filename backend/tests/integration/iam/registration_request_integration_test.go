package iamintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	modelbase "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRegistrationRequestFlow(t *testing.T) {
	t.Run("waitlist submit does not create tenant", func(t *testing.T) {
		db := setupRegistrationRequestDB(t)
		createRegistrationRequestPolicy(t, db, modeliam.RegistrationPolicyModeWaitlist)
		req, err := authsvc.NewRegistrationRequestService(db).Submit(context.Background(), authsvc.RegistrationRequestSubmitInput{
			TenantName: "Waitlist Tenant",
			OwnerEmail: "waitlist@example.com",
			Channel:    "internal_beta",
		})
		require.NoError(t, err)
		require.Equal(t, modeliam.RegistrationRequestModeWaitlist, req.Mode)
		require.Equal(t, modeliam.RegistrationRequestStatusSubmitted, req.Status)

		var tenantCount int64
		require.NoError(t, db.Model(&modeltenant.Tenant{}).Count(&tenantCount).Error)
		require.EqualValues(t, 0, tenantCount)
	})

	t.Run("approval approve creates tenant and converts request", func(t *testing.T) {
		db := setupRegistrationRequestDB(t)
		createRegistrationRequestPolicy(t, db, modeliam.RegistrationPolicyModeApprovalRequired)
		svc := authsvc.NewRegistrationRequestService(db, authsvc.WithRegistrationRequestTenantCreator(fakeRegistrationTenantCreator{}))
		req, err := svc.Submit(context.Background(), authsvc.RegistrationRequestSubmitInput{
			TenantName: "Approval Tenant",
			TenantKey:  "approval-tenant",
			OwnerEmail: "approval@example.com",
			Channel:    "internal_beta",
		})
		require.NoError(t, err)

		approved, err := svc.Approve(context.Background(), authsvc.RegistrationRequestReviewInput{
			RequestUUID:      req.UUID.String(),
			ReviewerUserUUID: uuid.NewString(),
		})
		require.NoError(t, err)
		require.Equal(t, modeliam.RegistrationRequestStatusConverted, approved.Status)
		require.NotEmpty(t, approved.CreatedTenantUUID)

		var tenant modeltenant.Tenant
		require.NoError(t, db.Where("uuid = ?", approved.CreatedTenantUUID).First(&tenant).Error)
		require.Equal(t, "Approval Tenant", tenant.Name)
	})

	t.Run("reject keeps reason code", func(t *testing.T) {
		db := setupRegistrationRequestDB(t)
		createRegistrationRequestPolicy(t, db, modeliam.RegistrationPolicyModeApprovalRequired)
		svc := authsvc.NewRegistrationRequestService(db)
		req, err := svc.Submit(context.Background(), authsvc.RegistrationRequestSubmitInput{
			TenantName: "Reject Tenant",
			OwnerEmail: "reject@example.com",
		})
		require.NoError(t, err)

		rejected, err := svc.Reject(context.Background(), authsvc.RegistrationRequestReviewInput{
			RequestUUID:      req.UUID.String(),
			ReviewerUserUUID: uuid.NewString(),
			RejectReasonCode: "not_fit_beta",
		})
		require.NoError(t, err)
		require.Equal(t, modeliam.RegistrationRequestStatusRejected, rejected.Status)
		require.Equal(t, "not_fit_beta", rejected.RejectReasonCode)

		var audit modeliam.RegistrationPolicyAuditEvent
		require.NoError(t, db.Where("request_uuid = ? AND event_type = ?", rejected.UUID.String(), modeliam.RegistrationPolicyAuditEventRequestRejected).First(&audit).Error)
		require.Equal(t, modeliam.RegistrationPolicyAuditDecisionDeny, audit.Decision)
		require.Equal(t, "not_fit_beta", audit.ReasonCode)
	})
}

type fakeRegistrationTenantCreator struct{}

func (fakeRegistrationTenantCreator) CreateTenantFromRegistrationRequest(ctx context.Context, tx *gorm.DB, req *modeliam.RegistrationRequest) (string, error) {
	tenant := &modeltenant.Tenant{
		PowerUUIDModel: modelbase.PowerUUIDModel{UUID: uuid.New()},
		Key:            req.TenantKey,
		Name:           req.TenantName,
		Domain:         req.TenantKey + ".tenant.powerx.local",
		Plan:           req.Plan,
		Type:           modeltenant.TenantTypeEnterprise,
		Status:         modeltenant.TenantStatusActive,
	}
	if tenant.Key == "" {
		tenant.Key = "tenant-" + tenant.UUID.String()[:8]
		tenant.Domain = tenant.Key + ".tenant.powerx.local"
	}
	if err := tx.WithContext(ctx).Create(tenant).Error; err != nil {
		return "", err
	}
	return tenant.UUID.String(), nil
}

func setupRegistrationRequestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared&_loc=UTC", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	prevSchema := modelbase.PowerXSchema
	modelbase.PowerXSchema = "main"
	t.Cleanup(func() { modelbase.PowerXSchema = prevSchema })
	require.NoError(t, db.AutoMigrate(&modeliam.RegistrationPolicy{}, &modeliam.RegistrationRequest{}, &modeliam.RegistrationPolicyAuditEvent{}, &modeltenant.Tenant{}))
	return db
}

func createRegistrationRequestPolicy(t *testing.T, db *gorm.DB, mode string) {
	t.Helper()
	raw, err := json.Marshal([]map[string]any{})
	require.NoError(t, err)
	now := time.Now().UTC()
	require.NoError(t, db.Create(&modeliam.RegistrationPolicy{
		PowerUUIDModel:    modelbase.PowerUUIDModel{UUID: uuid.New()},
		Version:           1,
		Mode:              mode,
		Status:            modeliam.RegistrationPolicyStatusActive,
		Rules:             datatypes.JSON(raw),
		ActivatedAt:       &now,
		CreatedByUserUUID: uuid.NewString(),
		UpdatedByUserUUID: uuid.NewString(),
	}).Error)
}
