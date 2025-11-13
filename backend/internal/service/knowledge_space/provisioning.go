package knowledge_space

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
)

// CreateSpace provisions a knowledge space with IAM blocking state.
func (s *Service) CreateSpace(ctx context.Context, in CreateSpaceInput) (*models.KnowledgeSpace, error) {
	if err := s.validateCreateInput(in); err != nil {
		return nil, err
	}

	release, err := s.acquireTenantLock(ctx, in.TenantID)
	if err != nil {
		return nil, err
	}
	defer release()

	start := s.clock()
	var created *models.KnowledgeSpace
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		spaces, policies, iamRepo, _ := s.repositories(tx)

		existing, err := spaces.FindByTenantAndName(ctx, in.TenantID, strings.TrimSpace(in.SpaceName))
		if err != nil {
			return err
		}
		if existing != nil {
			return ErrSpaceConflict
		}

		if in.PolicyVersion == 0 {
			return ErrInvalidInput
		}
		tpl, err := policies.GetByID(ctx, in.PolicyVersion)
		if err != nil {
			return err
		}
		if tpl == nil {
			return ErrInvalidInput
		}

		ff := normalizeFeatureFlags(in.FeatureFlags)
		rawFlags, err := json.Marshal(ff)
		if err != nil {
			return err
		}

		space := &models.KnowledgeSpace{
			TenantID:                in.TenantID,
			SpaceName:               strings.TrimSpace(in.SpaceName),
			DepartmentCode:          strings.ToUpper(strings.TrimSpace(in.DepartmentCode)),
			Status:                  models.KnowledgeSpaceStatusPending,
			QuotaCPU:                in.QuotaCPU,
			QuotaStorageGB:          in.QuotaStorageGB,
			PolicyTemplateVersionID: in.PolicyVersion,
			FeatureFlags:            datatypes.JSON(rawFlags),
			AuditToken:              "ks-" + uuid.NewString(),
			CreatedBy:               in.RequestedBy,
			UpdatedBy:               in.RequestedBy,
		}

		if err := tx.Create(space).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrSpaceConflict
			}
			return err
		}

		task := &models.IAMSyncTask{
			SpaceUUID: space.UUID,
			Provider:  "default",
			Status:    models.IAMSyncStatusPending,
		}
		if _, err := iamRepo.Create(ctx, task); err != nil {
			return err
		}

		if err := s.insertAuditTrail(ctx, tx, space, "provision.created", in.RequestedBy, map[string]any{
			"quota_cpu":        in.QuotaCPU,
			"quota_storage_gb": in.QuotaStorageGB,
			"policy_id":        in.PolicyVersion,
			"feature_flags":    ff,
		}); err != nil {
			return err
		}

		created = space
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrSpaceConflict) {
			return nil, err
		}
		if errors.Is(err, ErrInvalidInput) {
			return nil, err
		}
		return nil, err
	}

	s.inst.RecordProvisioning(true, s.clock().Sub(start))
	s.publishEvent(ctx, "created", created)
	return created, nil
}

// UpdateSpace mutates quotas/features/status.
func (s *Service) UpdateSpace(ctx context.Context, in UpdateSpaceInput) (*models.KnowledgeSpace, error) {
	if in.SpaceID == uuid.Nil {
		return nil, ErrInvalidInput
	}

	var updated *models.KnowledgeSpace
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		spaces, policies, _, _ := s.repositories(tx)
		space, err := spaces.FindByUUID(ctx, in.SpaceID)
		if err != nil {
			return err
		}
		if space == nil {
			return ErrSpaceNotFound
		}

		quotaCPUChanged := false
		if in.QuotaCPU > 0 {
			space.QuotaCPU = in.QuotaCPU
			quotaCPUChanged = true
		}
		quotaStorageChanged := false
		if in.QuotaStorageGB > 0 {
			space.QuotaStorageGB = in.QuotaStorageGB
			quotaStorageChanged = true
		}
		policyChanged := false
		if in.PolicyVersion > 0 && in.PolicyVersion != space.PolicyTemplateVersionID {
			tpl, err := policies.GetByID(ctx, in.PolicyVersion)
			if err != nil {
				return err
			}
			if tpl == nil {
				return ErrInvalidInput
			}
			space.PolicyTemplateVersionID = in.PolicyVersion
			policyChanged = true
		}
		featureChanged := false
		if len(in.FeatureFlags) > 0 {
			flags := normalizeFeatureFlags(in.FeatureFlags)
			raw, err := json.Marshal(flags)
			if err != nil {
				return err
			}
			space.FeatureFlags = datatypes.JSON(raw)
			featureChanged = true
		}
		statusChanged := false
		if strings.TrimSpace(in.Status) != "" && strings.TrimSpace(in.Status) != space.Status {
			if !isValidTransition(space.Status, in.Status) {
				return ErrInvalidStatusTransition
			}
			space.Status = strings.TrimSpace(in.Status)
			statusChanged = true
		}
		if in.UpdatedBy != "" {
			space.UpdatedBy = in.UpdatedBy
		}
		updates := map[string]any{}
		if quotaCPUChanged {
			updates["quota_cpu"] = space.QuotaCPU
		}
		if quotaStorageChanged {
			updates["quota_storage_gb"] = space.QuotaStorageGB
		}
		if policyChanged {
			updates["policy_template_version_id"] = space.PolicyTemplateVersionID
		}
		if featureChanged {
			updates["feature_flags"] = space.FeatureFlags
		}
		if statusChanged {
			updates["status"] = space.Status
		}
		if in.UpdatedBy != "" {
			updates["updated_by"] = space.UpdatedBy
		}
		if len(updates) > 0 {
			if err := tx.Model(&models.KnowledgeSpace{}).
				Where("uuid = ?", space.UUID).
				Updates(updates).Error; err != nil {
				return err
			}
		}

		if err := s.insertAuditTrail(ctx, tx, space, "provision.updated", in.UpdatedBy, map[string]any{
			"status":    space.Status,
			"quota_cpu": space.QuotaCPU,
		}); err != nil {
			return err
		}

		updated = space
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.publishEvent(ctx, "updated", updated)
	return updated, nil
}

// RetireSpace moves a space into read-only retention mode.
func (s *Service) RetireSpace(ctx context.Context, in RetireSpaceInput) (*models.KnowledgeSpace, error) {
	if in.SpaceID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	now := s.clock()
	var retired *models.KnowledgeSpace
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		spaces, _, _, _ := s.repositories(tx)
		space, err := spaces.FindByUUID(ctx, in.SpaceID)
		if err != nil {
			return err
		}
		if space == nil {
			return ErrSpaceNotFound
		}
		if space.Status == models.KnowledgeSpaceStatusRetired {
			retired = space
			return nil
		}
		space.Status = models.KnowledgeSpaceStatusRetired
		space.RetireAt = &now
		expire := s.retentionDeadline(now)
		space.RetentionExpiresAt = &expire
		if in.RequestedBy != "" {
			space.UpdatedBy = in.RequestedBy
		}

		updates := map[string]any{
			"status":               space.Status,
			"retire_at":            space.RetireAt,
			"retention_expires_at": space.RetentionExpiresAt,
		}
		if in.RequestedBy != "" {
			updates["updated_by"] = space.UpdatedBy
		}
		if err := tx.Model(&models.KnowledgeSpace{}).
			Where("uuid = ?", space.UUID).
			Updates(updates).Error; err != nil {
			return err
		}

		if err := s.insertAuditTrail(ctx, tx, space, "provision.retired", in.RequestedBy, map[string]any{
			"reason":               in.Reason,
			"retire_at":            now,
			"retention_expires_at": expire,
		}); err != nil {
			return err
		}

		retired = space
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.publishEvent(ctx, "retired", retired)
	return retired, nil
}

func (s *Service) validateCreateInput(in CreateSpaceInput) error {
	if in.TenantID == uuid.Nil ||
		strings.TrimSpace(in.SpaceName) == "" ||
		strings.TrimSpace(in.DepartmentCode) == "" ||
		in.QuotaCPU <= 0 ||
		in.QuotaStorageGB < 50 {
		return ErrInvalidInput
	}
	return nil
}

func normalizeFeatureFlags(flags []string) []string {
	seen := make(map[string]struct{}, len(flags))
	out := make([]string, 0, len(flags))
	for _, f := range flags {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		lower := strings.ToLower(f)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		out = append(out, lower)
	}
	return out
}

func isValidTransition(current, next string) bool {
	switch current {
	case models.KnowledgeSpaceStatusDraft:
		return next == models.KnowledgeSpaceStatusPending || next == models.KnowledgeSpaceStatusActive
	case models.KnowledgeSpaceStatusPending:
		return next == models.KnowledgeSpaceStatusActive || next == models.KnowledgeSpaceStatusDraft
	case models.KnowledgeSpaceStatusActive:
		return next == models.KnowledgeSpaceStatusPending
	default:
		return false
	}
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique")
}

func (s *Service) acquireTenantLock(ctx context.Context, tenant uuid.UUID) (func(), error) {
	if s.redis == nil {
		return s.acquireLocalLock(tenant), nil
	}
	token := uuid.NewString()
	key := s.lockKey(tenant)
	ok, err := s.redis.SetNX(ctx, key, token, s.lockTTL).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrProvisioningBusy
	}
	release := func() {
		script := redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`)
		_ = script.Run(ctx, s.redis, []string{key}, token).Err()
	}
	return release, nil
}

func computePayloadHash(payload map[string]any) string {
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
