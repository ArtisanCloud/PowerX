package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrRegistrationInviteServiceNotConfigured = errors.New("registration invite service not configured")
	ErrRegistrationInviteInvalid              = errors.New("registration invite invalid")
	ErrRegistrationInviteUnavailable          = errors.New("registration invite unavailable")
	ErrRegistrationInviteConsumed             = errors.New("registration invite consumed")
)

type InviteCodeService struct {
	DB  *gorm.DB
	now func() time.Time
}

type InviteCodeServiceOption func(*InviteCodeService)

func NewInviteCodeService(db *gorm.DB, opts ...InviteCodeServiceOption) *InviteCodeService {
	s := &InviteCodeService{DB: db, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithInviteCodeClock(now func() time.Time) InviteCodeServiceOption {
	return func(s *InviteCodeService) {
		if now != nil {
			s.now = now
		}
	}
}

type InviteBatchCreateInput struct {
	Name                string
	MaxCodes            int
	MaxUsesPerCode      int
	AllowedPlan         string
	AllowedEmailDomains []string
	AllowedChannels     []string
	StartsAt            *time.Time
	ExpiresAt           *time.Time
	ActorUserUUID       string
}

type InviteCodeConsumeInput struct {
	Code              string
	Contact           string
	Email             string
	Channel           string
	Plan              string
	TenantUUID        string
	IdempotencyKey    string
	ExpectedBatchUUID string
}

func (s *InviteCodeService) CreateBatch(ctx context.Context, in InviteBatchCreateInput) (*modeliam.RegistrationInviteBatch, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationInviteServiceNotConfigured
	}
	if strings.TrimSpace(in.Name) == "" || in.MaxCodes <= 0 {
		return nil, fmt.Errorf("%w: name and max_codes required", ErrRegistrationInviteInvalid)
	}
	maxUses := in.MaxUsesPerCode
	if maxUses <= 0 {
		maxUses = 1
	}
	domains, err := json.Marshal(normalizeStringSet(in.AllowedEmailDomains))
	if err != nil {
		return nil, err
	}
	channels, err := json.Marshal(normalizeStringSet(in.AllowedChannels))
	if err != nil {
		return nil, err
	}
	batch := &modeliam.RegistrationInviteBatch{
		Name:                strings.TrimSpace(in.Name),
		Status:              modeliam.RegistrationInviteBatchStatusActive,
		MaxCodes:            in.MaxCodes,
		MaxUsesPerCode:      maxUses,
		AllowedPlan:         strings.TrimSpace(in.AllowedPlan),
		AllowedEmailDomains: datatypes.JSON(domains),
		AllowedChannels:     datatypes.JSON(channels),
		StartsAt:            in.StartsAt,
		ExpiresAt:           in.ExpiresAt,
		CreatedByUserUUID:   strings.TrimSpace(in.ActorUserUUID),
		UpdatedByUserUUID:   strings.TrimSpace(in.ActorUserUUID),
	}
	if err := s.DB.WithContext(ctx).Create(batch).Error; err != nil {
		return nil, err
	}
	return batch, nil
}

func (s *InviteCodeService) ListBatches(ctx context.Context, limit int) ([]modeliam.RegistrationInviteBatch, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationInviteServiceNotConfigured
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var items []modeliam.RegistrationInviteBatch
	if err := s.DB.WithContext(ctx).Order("created_at DESC, id DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *InviteCodeService) ListCodes(ctx context.Context, batchUUID string, limit int) ([]modeliam.RegistrationInviteCode, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationInviteServiceNotConfigured
	}
	batchUUID = strings.TrimSpace(batchUUID)
	if batchUUID == "" {
		return nil, fmt.Errorf("%w: batch_uuid required", ErrRegistrationInviteInvalid)
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	var batch modeliam.RegistrationInviteBatch
	if err := s.DB.WithContext(ctx).Where("uuid = ?", batchUUID).First(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRegistrationInviteUnavailable
		}
		return nil, err
	}
	var items []modeliam.RegistrationInviteCode
	if err := s.DB.WithContext(ctx).
		Where("batch_uuid = ?", batchUUID).
		Order("created_at ASC, id ASC").
		Limit(limit).
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *InviteCodeService) DeleteBatches(ctx context.Context, batchUUIDs []string) (int64, error) {
	if s == nil || s.DB == nil {
		return 0, ErrRegistrationInviteServiceNotConfigured
	}
	normalized := make([]string, 0, len(batchUUIDs))
	seen := map[string]struct{}{}
	for _, raw := range batchUUIDs {
		batchUUID := strings.TrimSpace(raw)
		if batchUUID == "" {
			continue
		}
		if _, err := uuid.Parse(batchUUID); err != nil {
			return 0, fmt.Errorf("%w: invalid batch_uuid", ErrRegistrationInviteInvalid)
		}
		if _, ok := seen[batchUUID]; ok {
			continue
		}
		seen[batchUUID] = struct{}{}
		normalized = append(normalized, batchUUID)
	}
	if len(normalized) == 0 {
		return 0, fmt.Errorf("%w: batch_uuid required", ErrRegistrationInviteInvalid)
	}

	var deleted int64
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing int64
		if err := tx.Model(&modeliam.RegistrationInviteBatch{}).
			Where("uuid IN ?", normalized).
			Count(&existing).Error; err != nil {
			return err
		}
		if existing != int64(len(normalized)) {
			return ErrRegistrationInviteUnavailable
		}

		var used int64
		if err := tx.Model(&modeliam.RegistrationInviteCode{}).
			Where("batch_uuid IN ? AND use_count > 0", normalized).
			Count(&used).Error; err != nil {
			return err
		}
		if used > 0 {
			return fmt.Errorf("%w: used invite codes cannot be deleted", ErrRegistrationInviteInvalid)
		}

		if err := tx.Where("batch_uuid IN ?", normalized).
			Delete(&modeliam.RegistrationInviteCode{}).Error; err != nil {
			return err
		}
		result := tx.Where("uuid IN ?", normalized).Delete(&modeliam.RegistrationInviteBatch{})
		if result.Error != nil {
			return result.Error
		}
		deleted = result.RowsAffected
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *InviteCodeService) ResetMissingPlainCodes(ctx context.Context, batchUUID string) ([]modeliam.RegistrationInviteCode, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationInviteServiceNotConfigured
	}
	batchUUID = strings.TrimSpace(batchUUID)
	if batchUUID == "" {
		return nil, fmt.Errorf("%w: batch_uuid required", ErrRegistrationInviteInvalid)
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch modeliam.RegistrationInviteBatch
		if err := tx.Where("uuid = ?", batchUUID).First(&batch).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRegistrationInviteUnavailable
			}
			return err
		}
		var items []modeliam.RegistrationInviteCode
		if err := tx.
			Where("batch_uuid = ? AND plain_code = ? AND use_count = 0 AND status = ?", batchUUID, "", modeliam.RegistrationInviteCodeStatusActive).
			Order("created_at ASC, id ASC").
			Find(&items).Error; err != nil {
			return err
		}
		for i := range items {
			var lastErr error
			for attempt := 0; attempt < 5; attempt++ {
				code, err := randomInviteCode()
				if err != nil {
					return err
				}
				lastErr = tx.Model(&items[i]).Updates(map[string]any{
					"plain_code": code,
					"code_hash":  HashInviteCode(code),
				}).Error
				if lastErr == nil {
					break
				}
			}
			if lastErr != nil {
				return lastErr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ListCodes(ctx, batchUUID, 1000)
}

func (s *InviteCodeService) SetBatchStatus(ctx context.Context, batchUUID string, status string, actorUserUUID string) (*modeliam.RegistrationInviteBatch, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationInviteServiceNotConfigured
	}
	batchUUID = strings.TrimSpace(batchUUID)
	status = strings.TrimSpace(status)
	switch status {
	case modeliam.RegistrationInviteBatchStatusActive,
		modeliam.RegistrationInviteBatchStatusPaused,
		modeliam.RegistrationInviteBatchStatusRevoked:
	default:
		return nil, fmt.Errorf("%w: invalid batch status", ErrRegistrationInviteInvalid)
	}
	var batch modeliam.RegistrationInviteBatch
	if err := s.DB.WithContext(ctx).Where("uuid = ?", batchUUID).First(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRegistrationInviteUnavailable
		}
		return nil, err
	}
	batch.Status = status
	batch.UpdatedByUserUUID = strings.TrimSpace(actorUserUUID)
	if err := s.DB.WithContext(ctx).Save(&batch).Error; err != nil {
		return nil, err
	}
	return &batch, nil
}

func (s *InviteCodeService) GenerateCodes(ctx context.Context, batchUUID string, count int) ([]string, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationInviteServiceNotConfigured
	}
	batchUUID = strings.TrimSpace(batchUUID)
	if batchUUID == "" || count <= 0 {
		return nil, fmt.Errorf("%w: batch_uuid and count required", ErrRegistrationInviteInvalid)
	}
	var batch modeliam.RegistrationInviteBatch
	if err := s.DB.WithContext(ctx).Where("uuid = ?", batchUUID).First(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRegistrationInviteUnavailable
		}
		return nil, err
	}
	var existing int64
	if err := s.DB.WithContext(ctx).Model(&modeliam.RegistrationInviteCode{}).Where("batch_uuid = ?", batchUUID).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing+int64(count) > int64(batch.MaxCodes) {
		return nil, fmt.Errorf("%w: max_codes exceeded", ErrRegistrationInviteInvalid)
	}
	plain := make([]string, 0, count)
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for len(plain) < count {
			code, err := randomInviteCode()
			if err != nil {
				return err
			}
			record := &modeliam.RegistrationInviteCode{
				BatchUUID: batchUUID,
				PlainCode: code,
				CodeHash:  HashInviteCode(code),
				Status:    modeliam.RegistrationInviteCodeStatusActive,
				MaxUses:   batch.MaxUsesPerCode,
			}
			if err := tx.Create(record).Error; err != nil {
				continue
			}
			plain = append(plain, code)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return plain, nil
}

func (s *InviteCodeService) Consume(ctx context.Context, tx *gorm.DB, in InviteCodeConsumeInput) (*modeliam.RegistrationInviteCode, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationInviteServiceNotConfigured
	}
	db := tx
	if db == nil {
		db = s.DB
	}
	codeHash := HashInviteCode(in.Code)
	if codeHash == "" {
		return nil, fmt.Errorf("%w: code required", ErrRegistrationInviteInvalid)
	}
	var code modeliam.RegistrationInviteCode
	if err := db.WithContext(ctx).Where("code_hash = ?", codeHash).First(&code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRegistrationInviteUnavailable
		}
		return nil, err
	}
	if strings.TrimSpace(in.ExpectedBatchUUID) != "" && code.BatchUUID != strings.TrimSpace(in.ExpectedBatchUUID) {
		return nil, ErrRegistrationInviteUnavailable
	}
	if code.Status != modeliam.RegistrationInviteCodeStatusActive {
		return nil, ErrRegistrationInviteUnavailable
	}
	if code.MaxUses <= 0 || code.UseCount >= code.MaxUses {
		return nil, ErrRegistrationInviteConsumed
	}
	var batch modeliam.RegistrationInviteBatch
	if err := db.WithContext(ctx).Where("uuid = ?", code.BatchUUID).First(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRegistrationInviteUnavailable
		}
		return nil, err
	}
	if err := s.validateBatch(batch, in); err != nil {
		return nil, err
	}
	now := s.now().UTC()
	code.UseCount++
	code.LastUsedAt = &now
	code.LastUsedByContactHash = hashContact(firstNonEmpty(in.Contact, in.Email))
	code.ConsumedTenantUUID = strings.TrimSpace(in.TenantUUID)
	if code.UseCount >= code.MaxUses {
		code.Status = modeliam.RegistrationInviteCodeStatusConsumed
	}
	if err := db.WithContext(ctx).Save(&code).Error; err != nil {
		return nil, err
	}
	return &code, nil
}

func (s *InviteCodeService) validateBatch(batch modeliam.RegistrationInviteBatch, in InviteCodeConsumeInput) error {
	switch batch.Status {
	case modeliam.RegistrationInviteBatchStatusActive:
	default:
		return ErrRegistrationInviteUnavailable
	}
	now := s.now()
	if batch.StartsAt != nil && now.Before(*batch.StartsAt) {
		return ErrRegistrationInviteUnavailable
	}
	if batch.ExpiresAt != nil && now.After(*batch.ExpiresAt) {
		return ErrRegistrationInviteUnavailable
	}
	if batch.AllowedPlan != "" && strings.TrimSpace(in.Plan) != "" && batch.AllowedPlan != strings.TrimSpace(in.Plan) {
		return ErrRegistrationInviteUnavailable
	}
	if !jsonStringSetEmpty(batch.AllowedEmailDomains) && !jsonStringSetContains(batch.AllowedEmailDomains, emailDomain(in.Email)) {
		return ErrRegistrationInviteUnavailable
	}
	if !jsonStringSetEmpty(batch.AllowedChannels) && !jsonStringSetContains(batch.AllowedChannels, in.Channel) {
		return ErrRegistrationInviteUnavailable
	}
	return nil
}

func HashInviteCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func hashContact(contact string) string {
	contact = strings.ToLower(strings.TrimSpace(contact))
	if contact == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(contact))
	return hex.EncodeToString(sum[:])
}

func randomInviteCode() (string, error) {
	var raw [10]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "PX-" + strings.ToUpper(hex.EncodeToString(raw[:])), nil
}

func normalizeStringSet(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func jsonStringSetEmpty(raw datatypes.JSON) bool {
	values := decodeJSONStringSet(raw)
	return len(values) == 0
}

func jsonStringSetContains(raw datatypes.JSON, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return false
	}
	for _, value := range decodeJSONStringSet(raw) {
		if value == needle {
			return true
		}
	}
	return false
}

func decodeJSONStringSet(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return normalizeStringSet(values)
}
