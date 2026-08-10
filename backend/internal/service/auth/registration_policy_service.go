package auth

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"gorm.io/gorm"
)

var (
	ErrRegistrationPolicyServiceNotConfigured = errors.New("registration policy service not configured")
	ErrRegistrationPolicyActiveMissing        = errors.New("active registration policy missing")
	ErrRegistrationPolicyInvalid              = errors.New("registration policy invalid")
	ErrRegistrationPolicyRejected             = errors.New("registration policy rejected")
)

type RegistrationPolicyService struct {
	DB  *gorm.DB
	now func() time.Time
}

type RegistrationPolicyServiceOption func(*RegistrationPolicyService)

func NewRegistrationPolicyService(db *gorm.DB, opts ...RegistrationPolicyServiceOption) *RegistrationPolicyService {
	s := &RegistrationPolicyService{DB: db, now: time.Now}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithRegistrationPolicyClock(now func() time.Time) RegistrationPolicyServiceOption {
	return func(s *RegistrationPolicyService) {
		if now != nil {
			s.now = now
		}
	}
}

type RegistrationPolicyEvaluateInput struct {
	Email      string
	Phone      string
	InviteCode string
	Channel    string
	Campaign   string
}

type RegistrationPolicyRuleInput struct {
	Type      string   `json:"type"`
	Values    []string `json:"values,omitempty"`
	BatchUUID string   `json:"batch_uuid,omitempty"`
	Value     *float64 `json:"value,omitempty"`
	Seed      string   `json:"seed,omitempty"`
}

type RegistrationPolicyUpsertInput struct {
	Mode                 string
	RequiresVerification bool
	RequiresInviteCode   bool
	RequiresRootApproval bool
	DailyTenantQuota     *int
	TotalTenantQuota     *int
	StartAt              *time.Time
	EndAt                *time.Time
	Rules                []RegistrationPolicyRuleInput
	ActorUserUUID        string
}

type RegistrationPolicyEvaluation struct {
	PolicyUUID           string
	PolicyVersion        int
	Mode                 string
	CanSignup            bool
	CanSubmitRequest     bool
	RequiresVerification bool
	RequiresInviteCode   bool
	RequiresRootApproval bool
	ReasonCode           string
}

type registrationPolicyRule struct {
	Type      string   `json:"type"`
	Values    []string `json:"values"`
	BatchUUID string   `json:"batch_uuid"`
	Value     *float64 `json:"value"`
	Seed      string   `json:"seed"`
}

func (s *RegistrationPolicyService) Evaluate(ctx context.Context, in RegistrationPolicyEvaluateInput) (*RegistrationPolicyEvaluation, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationPolicyServiceNotConfigured
	}
	var policy modeliam.RegistrationPolicy
	if err := s.DB.WithContext(ctx).
		Where("status = ?", modeliam.RegistrationPolicyStatusActive).
		Order("version DESC, id DESC").
		First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRegistrationPolicyActiveMissing
		}
		return nil, err
	}
	return s.evaluatePolicy(ctx, &policy, normalizeRegistrationPolicyInput(in))
}

func (s *RegistrationPolicyService) GetActive(ctx context.Context) (*modeliam.RegistrationPolicy, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationPolicyServiceNotConfigured
	}
	var policy modeliam.RegistrationPolicy
	if err := s.DB.WithContext(ctx).
		Where("status = ?", modeliam.RegistrationPolicyStatusActive).
		Order("version DESC, id DESC").
		First(&policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRegistrationPolicyActiveMissing
		}
		return nil, err
	}
	return &policy, nil
}

func (s *RegistrationPolicyService) ListHistory(ctx context.Context, limit int) ([]modeliam.RegistrationPolicy, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationPolicyServiceNotConfigured
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var items []modeliam.RegistrationPolicy
	if err := s.DB.WithContext(ctx).
		Order("version DESC, created_at DESC, id DESC").
		Limit(limit).
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (s *RegistrationPolicyService) CreateDraft(ctx context.Context, in RegistrationPolicyUpsertInput) (*modeliam.RegistrationPolicy, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationPolicyServiceNotConfigured
	}
	if err := validateRegistrationPolicyMode(in.Mode); err != nil {
		return nil, err
	}
	rules := make([]registrationPolicyRule, 0, len(in.Rules))
	for _, rule := range in.Rules {
		rules = append(rules, registrationPolicyRule{
			Type:      rule.Type,
			Values:    rule.Values,
			BatchUUID: rule.BatchUUID,
			Value:     rule.Value,
			Seed:      rule.Seed,
		})
	}
	if err := validateRegistrationPolicyRules(rules); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return nil, err
	}
	var maxVersion int
	if err := s.DB.WithContext(ctx).Model(&modeliam.RegistrationPolicy{}).Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
		return nil, err
	}
	policy := &modeliam.RegistrationPolicy{
		Version:              maxVersion + 1,
		Mode:                 in.Mode,
		Status:               modeliam.RegistrationPolicyStatusDraft,
		RequiresVerification: in.RequiresVerification,
		RequiresInviteCode:   in.RequiresInviteCode || in.Mode == modeliam.RegistrationPolicyModeInviteOnly,
		RequiresRootApproval: in.RequiresRootApproval || in.Mode == modeliam.RegistrationPolicyModeApprovalRequired,
		DailyTenantQuota:     in.DailyTenantQuota,
		TotalTenantQuota:     in.TotalTenantQuota,
		StartAt:              in.StartAt,
		EndAt:                in.EndAt,
		Rules:                raw,
		CreatedByUserUUID:    strings.TrimSpace(in.ActorUserUUID),
		UpdatedByUserUUID:    strings.TrimSpace(in.ActorUserUUID),
	}
	if err := s.DB.WithContext(ctx).Create(policy).Error; err != nil {
		return nil, err
	}
	return policy, nil
}

func (s *RegistrationPolicyService) Activate(ctx context.Context, policyUUID string, actorUserUUID string) (*modeliam.RegistrationPolicy, error) {
	if s == nil || s.DB == nil {
		return nil, ErrRegistrationPolicyServiceNotConfigured
	}
	policyUUID = strings.TrimSpace(policyUUID)
	if policyUUID == "" {
		return nil, fmt.Errorf("%w: policy_uuid required", ErrRegistrationPolicyInvalid)
	}
	var out modeliam.RegistrationPolicy
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var policy modeliam.RegistrationPolicy
		if err := tx.Where("uuid = ?", policyUUID).First(&policy).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRegistrationPolicyActiveMissing
			}
			return err
		}
		if err := validateRegistrationPolicyMode(policy.Mode); err != nil {
			return err
		}
		rules, err := decodeRegistrationPolicyRules(&policy)
		if err != nil {
			return err
		}
		if err := validateRegistrationPolicyRules(rules); err != nil {
			return err
		}
		now := s.now().UTC()
		if err := tx.Model(&modeliam.RegistrationPolicy{}).
			Where("status = ? AND uuid <> ?", modeliam.RegistrationPolicyStatusActive, policyUUID).
			Updates(map[string]any{
				"status":               modeliam.RegistrationPolicyStatusArchived,
				"updated_by_user_uuid": strings.TrimSpace(actorUserUUID),
			}).Error; err != nil {
			return err
		}
		policy.Status = modeliam.RegistrationPolicyStatusActive
		policy.ActivatedAt = &now
		policy.UpdatedByUserUUID = strings.TrimSpace(actorUserUUID)
		if err := tx.Save(&policy).Error; err != nil {
			return err
		}
		out = policy
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *RegistrationPolicyService) evaluatePolicy(ctx context.Context, policy *modeliam.RegistrationPolicy, in RegistrationPolicyEvaluateInput) (*RegistrationPolicyEvaluation, error) {
	if policy == nil {
		return nil, ErrRegistrationPolicyActiveMissing
	}
	rules, err := decodeRegistrationPolicyRules(policy)
	if err != nil {
		return nil, err
	}
	if err := validateRegistrationPolicyMode(policy.Mode); err != nil {
		return nil, err
	}
	if err := validateRegistrationPolicyRules(rules); err != nil {
		return nil, err
	}
	if !withinPolicyWindow(policy, s.now()) {
		return baseRegistrationPolicyEvaluation(policy, false, false, "outside_time_window"), nil
	}
	if ok, reason, err := s.evaluateQuotaRules(ctx, policy, rules); err != nil {
		return nil, err
	} else if !ok {
		return baseRegistrationPolicyEvaluation(policy, false, false, reason), nil
	}

	switch policy.Mode {
	case modeliam.RegistrationPolicyModeClosed:
		return baseRegistrationPolicyEvaluation(policy, false, false, "registration_closed"), nil
	case modeliam.RegistrationPolicyModeOpen:
		return baseRegistrationPolicyEvaluation(policy, true, false, ""), nil
	case modeliam.RegistrationPolicyModeInviteOnly:
		if strings.TrimSpace(in.InviteCode) == "" {
			return baseRegistrationPolicyEvaluation(policy, false, false, "invite_code_required"), nil
		}
		return baseRegistrationPolicyEvaluation(policy, true, false, ""), nil
	case modeliam.RegistrationPolicyModeWaitlist:
		return baseRegistrationPolicyEvaluation(policy, false, true, "waitlist_required"), nil
	case modeliam.RegistrationPolicyModeApprovalRequired:
		out := baseRegistrationPolicyEvaluation(policy, false, true, "approval_required")
		out.RequiresRootApproval = true
		return out, nil
	case modeliam.RegistrationPolicyModeAllowlist:
		if matchesAllowlist(rules, in) {
			return baseRegistrationPolicyEvaluation(policy, true, false, ""), nil
		}
		return baseRegistrationPolicyEvaluation(policy, false, false, "allowlist_miss"), nil
	case modeliam.RegistrationPolicyModeProgressiveRollout:
		if ok, reason := evaluateProgressiveRules(policy, rules, in, s.now()); !ok {
			return baseRegistrationPolicyEvaluation(policy, false, false, reason), nil
		}
		return baseRegistrationPolicyEvaluation(policy, true, false, ""), nil
	default:
		return nil, fmt.Errorf("%w: unknown mode %q", ErrRegistrationPolicyInvalid, policy.Mode)
	}
}

func baseRegistrationPolicyEvaluation(policy *modeliam.RegistrationPolicy, canSignup bool, canSubmitRequest bool, reason string) *RegistrationPolicyEvaluation {
	return &RegistrationPolicyEvaluation{
		PolicyUUID:           policy.UUID.String(),
		PolicyVersion:        policy.Version,
		Mode:                 policy.Mode,
		CanSignup:            canSignup,
		CanSubmitRequest:     canSubmitRequest,
		RequiresVerification: policy.RequiresVerification,
		RequiresInviteCode:   policy.RequiresInviteCode || policy.Mode == modeliam.RegistrationPolicyModeInviteOnly,
		RequiresRootApproval: policy.RequiresRootApproval || policy.Mode == modeliam.RegistrationPolicyModeApprovalRequired,
		ReasonCode:           reason,
	}
}

func decodeRegistrationPolicyRules(policy *modeliam.RegistrationPolicy) ([]registrationPolicyRule, error) {
	if policy == nil || len(policy.Rules) == 0 {
		return nil, nil
	}
	var rules []registrationPolicyRule
	if err := json.Unmarshal(policy.Rules, &rules); err != nil {
		return nil, fmt.Errorf("%w: decode rules: %v", ErrRegistrationPolicyInvalid, err)
	}
	return rules, nil
}

func validateRegistrationPolicyMode(mode string) error {
	switch mode {
	case modeliam.RegistrationPolicyModeClosed,
		modeliam.RegistrationPolicyModeOpen,
		modeliam.RegistrationPolicyModeInviteOnly,
		modeliam.RegistrationPolicyModeWaitlist,
		modeliam.RegistrationPolicyModeApprovalRequired,
		modeliam.RegistrationPolicyModeAllowlist,
		modeliam.RegistrationPolicyModeProgressiveRollout:
		return nil
	default:
		return fmt.Errorf("%w: unknown mode %q", ErrRegistrationPolicyInvalid, mode)
	}
}

func validateRegistrationPolicyRules(rules []registrationPolicyRule) error {
	for _, rule := range rules {
		switch rule.Type {
		case modeliam.RegistrationPolicyRuleEmailDomainAllowlist,
			modeliam.RegistrationPolicyRuleContactAllowlist,
			modeliam.RegistrationPolicyRuleInviteBatch,
			modeliam.RegistrationPolicyRuleChannelAllowlist,
			modeliam.RegistrationPolicyRulePercentage,
			modeliam.RegistrationPolicyRuleTimeWindow,
			modeliam.RegistrationPolicyRuleDailyQuota,
			modeliam.RegistrationPolicyRuleTotalQuota:
		default:
			return fmt.Errorf("%w: unknown rule type %q", ErrRegistrationPolicyInvalid, rule.Type)
		}
		if rule.Type == modeliam.RegistrationPolicyRulePercentage {
			if rule.Value == nil || *rule.Value < 0 || *rule.Value > 100 {
				return fmt.Errorf("%w: percentage value out of range", ErrRegistrationPolicyInvalid)
			}
			if rule.Seed != "" && rule.Seed != "contact" && rule.Seed != "email" && rule.Seed != "phone" {
				return fmt.Errorf("%w: percentage seed %q", ErrRegistrationPolicyInvalid, rule.Seed)
			}
		}
	}
	return nil
}

func withinPolicyWindow(policy *modeliam.RegistrationPolicy, now time.Time) bool {
	if policy.StartAt != nil && now.Before(*policy.StartAt) {
		return false
	}
	if policy.EndAt != nil && now.After(*policy.EndAt) {
		return false
	}
	return true
}

func (s *RegistrationPolicyService) evaluateQuotaRules(ctx context.Context, policy *modeliam.RegistrationPolicy, rules []registrationPolicyRule) (bool, string, error) {
	totalQuota := intFromPtr(policy.TotalTenantQuota)
	dailyQuota := intFromPtr(policy.DailyTenantQuota)
	for _, rule := range rules {
		switch rule.Type {
		case modeliam.RegistrationPolicyRuleTotalQuota:
			totalQuota = intFromFloatPtr(rule.Value)
		case modeliam.RegistrationPolicyRuleDailyQuota:
			dailyQuota = intFromFloatPtr(rule.Value)
		}
	}
	if totalQuota > 0 {
		var count int64
		if err := s.DB.WithContext(ctx).Model(&modeltenant.Tenant{}).Count(&count).Error; err != nil {
			return false, "", err
		}
		if count >= int64(totalQuota) {
			return false, "total_quota_exceeded", nil
		}
	}
	if dailyQuota > 0 {
		start := s.now().Truncate(24 * time.Hour)
		var count int64
		if err := s.DB.WithContext(ctx).Model(&modeltenant.Tenant{}).Where("created_at >= ?", start).Count(&count).Error; err != nil {
			return false, "", err
		}
		if count >= int64(dailyQuota) {
			return false, "daily_quota_exceeded", nil
		}
	}
	return true, "", nil
}

func evaluateProgressiveRules(policy *modeliam.RegistrationPolicy, rules []registrationPolicyRule, in RegistrationPolicyEvaluateInput, now time.Time) (bool, string) {
	for _, rule := range rules {
		switch rule.Type {
		case modeliam.RegistrationPolicyRulePercentage:
			seed := rolloutSeed(rule, in)
			if seed == "" || stableRolloutBucket(policy.UUID.String(), policy.Version, seed) >= *rule.Value {
				return false, "rollout_percentage_miss"
			}
		case modeliam.RegistrationPolicyRuleTimeWindow:
			if !ruleTimeWindowContains(rule, now) {
				return false, "time_window_miss"
			}
		case modeliam.RegistrationPolicyRuleChannelAllowlist:
			if !containsNormalized(rule.Values, in.Channel) {
				return false, "channel_allowlist_miss"
			}
		case modeliam.RegistrationPolicyRuleEmailDomainAllowlist, modeliam.RegistrationPolicyRuleContactAllowlist:
			if !matchesAllowlist([]registrationPolicyRule{rule}, in) {
				return false, "allowlist_miss"
			}
		}
	}
	return true, ""
}

func matchesAllowlist(rules []registrationPolicyRule, in RegistrationPolicyEvaluateInput) bool {
	for _, rule := range rules {
		switch rule.Type {
		case modeliam.RegistrationPolicyRuleEmailDomainAllowlist:
			if containsNormalized(rule.Values, emailDomain(in.Email)) {
				return true
			}
		case modeliam.RegistrationPolicyRuleContactAllowlist:
			if containsNormalized(rule.Values, in.Email) || containsNormalized(rule.Values, in.Phone) {
				return true
			}
		case modeliam.RegistrationPolicyRuleChannelAllowlist:
			if containsNormalized(rule.Values, in.Channel) {
				return true
			}
		}
	}
	return false
}

func ruleTimeWindowContains(rule registrationPolicyRule, now time.Time) bool {
	if len(rule.Values) != 2 {
		return false
	}
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(rule.Values[0]))
	if err != nil {
		return false
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(rule.Values[1]))
	if err != nil {
		return false
	}
	return !now.Before(start) && !now.After(end)
}

func rolloutSeed(rule registrationPolicyRule, in RegistrationPolicyEvaluateInput) string {
	switch rule.Seed {
	case "", "contact":
		return firstNonEmpty(in.Email, in.Phone)
	case "email":
		return in.Email
	case "phone":
		return in.Phone
	default:
		return ""
	}
}

func stableRolloutBucket(policyUUID string, version int, seed string) float64 {
	sum := sha256.Sum256([]byte(policyUUID + ":" + strconv.Itoa(version) + ":" + strings.ToLower(strings.TrimSpace(seed))))
	n := binary.BigEndian.Uint64(sum[:8])
	return float64(n%10000) / 100
}

func normalizeRegistrationPolicyInput(in RegistrationPolicyEvaluateInput) RegistrationPolicyEvaluateInput {
	out := RegistrationPolicyEvaluateInput{
		Email:      strings.ToLower(strings.TrimSpace(in.Email)),
		Phone:      strings.TrimSpace(in.Phone),
		InviteCode: strings.TrimSpace(in.InviteCode),
		Channel:    strings.ToLower(strings.TrimSpace(in.Channel)),
		Campaign:   strings.TrimSpace(in.Campaign),
	}
	if out.Email != "" {
		if addr, err := mail.ParseAddress(out.Email); err == nil {
			out.Email = strings.ToLower(addr.Address)
		}
	}
	return out
}

func emailDomain(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return email[at+1:]
}

func containsNormalized(values []string, needle string) bool {
	needle = strings.ToLower(strings.TrimSpace(needle))
	if needle == "" {
		return false
	}
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == needle {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func intFromPtr(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func intFromFloatPtr(v *float64) int {
	if v == nil {
		return 0
	}
	return int(*v)
}
