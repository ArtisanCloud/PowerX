package capability_registry

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelregistry "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrVersionUpgradeForbidden           = errors.New("capability.upgrade_forbidden")
	ErrVersionUpgradeInvalid             = errors.New("capability.upgrade_invalid")
	ErrVersionUpgradeHashMismatch        = errors.New("capability.upgrade_hash_mismatch")
	ErrVersionUpgradeBindingMismatch     = errors.New("capability.upgrade_binding_mismatch")
	ErrVersionUpgradeNotFound            = errors.New("capability.upgrade_not_found")
	ErrVersionUpgradeUnavailable         = errors.New("capability.upgrade_unavailable")
	ErrVersionUpgradePersistenceFailed   = errors.New("capability.upgrade_persistence_failed")
	ErrVersionUpgradeRuntimeApplyFailed  = errors.New("capability.upgrade_runtime_apply_failed")
	ErrVersionUpgradeAuditFinalizeFailed = errors.New("capability.upgrade_audit_finalize_failed")
)

type VersionUpgradeStore interface {
	CurrentHash(context.Context, string, string) (string, bool)
	ConfirmUpgrade(context.Context, string, string, string) error
}

type VersionUpgradeService struct {
	db    *gorm.DB
	store VersionUpgradeStore
}

func NewVersionUpgradeService(db *gorm.DB, store VersionUpgradeStore) *VersionUpgradeService {
	return &VersionUpgradeService{db: db, store: store}
}

type VersionUpgradeInput struct{ TenantUUID, CapabilityID, CapabilitiesHash, Reason string }
type VersionUpgradeResult struct {
	TenantUUID       string `json:"tenant_uuid"`
	CapabilityID     string `json:"capability_id"`
	PreviousHash     string `json:"previous_hash"`
	CapabilitiesHash string `json:"capabilities_hash"`
	AuditID          uint64 `json:"audit_id"`
}

// Confirm requires a trusted human administrator. A durable approval audit is
// committed before changing the runtime lock. Retries can safely reapply the
// same approval if the runtime store failed after the audit was committed.
func (s *VersionUpgradeService) Confirm(ctx context.Context, in VersionUpgradeInput) (VersionUpgradeResult, error) {
	var out VersionUpgradeResult
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.UserID == 0 || claims.PluginID != "" || claims.CustomerUUID != "" {
		return out, ErrVersionUpgradeForbidden
	}
	admin := claims.IsRoot
	for _, role := range claims.Roles {
		if role == string(iam.CodeRoleAdmin) || role == string(iam.CodeSystemAdmin) {
			admin = true
		}
	}
	if !admin || (!claims.IsRoot && claims.TenantUUID != in.TenantUUID) {
		return out, ErrVersionUpgradeForbidden
	}
	tenant, err := uuid.Parse(in.TenantUUID)
	if err != nil || tenant == uuid.Nil {
		return out, ErrVersionUpgradeInvalid
	}
	raw, err := hex.DecodeString(in.CapabilitiesHash)
	if err != nil || len(raw) != 32 || strings.TrimSpace(in.CapabilityID) == "" || strings.TrimSpace(in.Reason) == "" || len(in.Reason) > 1024 {
		return out, ErrVersionUpgradeInvalid
	}
	if s == nil || s.db == nil || s.store == nil {
		return out, ErrVersionUpgradeUnavailable
	}
	in.TenantUUID = tenant.String()
	in.CapabilitiesHash = strings.ToLower(in.CapabilitiesHash)
	old, _ := s.store.CurrentHash(ctx, in.TenantUUID, in.CapabilityID)
	out = VersionUpgradeResult{TenantUUID: in.TenantUUID, CapabilityID: in.CapabilityID, PreviousHash: old, CapabilitiesHash: in.CapabilitiesHash}
	actor := int64(claims.UserID)
	meta, _ := json.Marshal(map[string]any{"reason": in.Reason, "actor_user_uuid": claims.UserUUID, "actor_member_uuid": claims.MemberUUID, "runtime_apply": "pending"})
	before, _ := json.Marshal(map[string]string{"capabilities_hash": old})
	after, _ := json.Marshal(map[string]string{"capabilities_hash": in.CapabilitiesHash})
	approval := modelaudit.AuditEvent{OccurredAt: time.Now().UTC(), TenantUUID: in.TenantUUID, CorrelationID: reqctx.GetTraceID(ctx), Source: "capability.version_upgrade", Operation: "CONFIRM_UPGRADE", ResourceType: "capability", ResourceID: in.CapabilityID, Outcome: "APPROVED", Severity: "INFO", ActorUserID: &actor, Meta: datatypes.JSON(meta), ChangesBefore: datatypes.JSON(before), ChangesAfter: datatypes.JSON(after)}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record modelregistry.CapabilityRecord
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("capability_id = ? AND status = ?", in.CapabilityID, "published").First(&record).Error; err != nil {
			return err
		}
		if record.CapabilitiesHash != in.CapabilitiesHash {
			return ErrVersionUpgradeHashMismatch
		}
		var registration modelregistry.CapabilityRegistration
		if err := tx.Where("tenant_uuid = ? AND capability_id = ?", in.TenantUUID, in.CapabilityID).Order("version DESC").First(&registration).Error; err != nil {
			return err
		}
		if registration.Status != "published" {
			return gorm.ErrRecordNotFound
		}
		if err := verifyMediaUpgradeBinding(tx, record, registration); err != nil {
			return err
		}
		return tx.Create(&approval).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return out, fmt.Errorf("%w: %w", ErrVersionUpgradeNotFound, err)
		}
		if errors.Is(err, ErrVersionUpgradeHashMismatch) || errors.Is(err, ErrVersionUpgradeBindingMismatch) {
			return out, err
		}
		return out, fmt.Errorf("%w: %w", ErrVersionUpgradePersistenceFailed, err)
	}
	out.AuditID = approval.ID
	// Never confirm before the approval audit has durably committed.
	if err = s.store.ConfirmUpgrade(ctx, in.TenantUUID, in.CapabilityID, in.CapabilitiesHash); err != nil {
		return out, fmt.Errorf("%w: %w", ErrVersionUpgradeRuntimeApplyFailed, err)
	}
	meta, _ = json.Marshal(map[string]any{"reason": in.Reason, "actor_user_uuid": claims.UserUUID, "actor_member_uuid": claims.MemberUUID, "runtime_apply": "confirmed"})
	err = s.db.WithContext(ctx).Model(&approval).Updates(map[string]any{"outcome": "SUCCESS", "meta": datatypes.JSON(meta)}).Error
	if err != nil {
		return out, fmt.Errorf("%w: %w", ErrVersionUpgradeAuditFinalizeFailed, err)
	}
	return out, nil
}

// 确认 hash 不能把尚未迁移的 HTTP 注册伪装成已升级的 typed binding。
func verifyMediaUpgradeBinding(tx *gorm.DB, record modelregistry.CapabilityRecord, registration modelregistry.CapabilityRegistration) error {
	media := isAIModelCatalogReadCapability(record.CapabilityID)
	switch record.CapabilityID {
	case "com.corex.ai.vlm.invoke", "com.corex.ai.image.invoke", "com.corex.ai.video.invoke", "com.corex.ai.tts.invoke", "com.corex.ai.media.tasks.read", "com.corex.ai.media.tasks.cancel":
		media = true
	}
	if !media {
		return nil
	}
	var protocols []modelregistry.ProtocolBinding
	if json.Unmarshal(record.Protocols, &protocols) != nil {
		return ErrVersionUpgradeBindingMismatch
	}
	endpoint := ""
	for _, binding := range protocols {
		if binding.Channel == "core_internal" && binding.Method == "INVOKE" && binding.ActorContext == "service_actor" {
			endpoint = binding.Endpoint
			break
		}
	}
	if endpoint == "" {
		return ErrVersionUpgradeBindingMismatch
	}
	var adapters []modelregistry.AdapterEndpoint
	if err := tx.Where("registration_id = ?", registration.ID).Find(&adapters).Error; err != nil {
		return err
	}
	if len(adapters) != 1 {
		return ErrVersionUpgradeBindingMismatch
	}
	ep := adapters[0]
	var labels map[string]string
	if json.Unmarshal(ep.Labels, &labels) != nil || ep.TransportType != "core_internal" || ep.Endpoint != endpoint || ep.ServiceRef != endpoint || labels["method"] != "INVOKE" {
		return ErrVersionUpgradeBindingMismatch
	}
	return nil
}
