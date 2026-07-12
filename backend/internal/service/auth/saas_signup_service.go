package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	pkgauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	modeliam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	repoiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	repotenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrSaaSSignupInvalidRequest     = errors.New("saas signup invalid request")
	ErrSaaSSignupTenantKeyExists    = errors.New("tenant key already exists")
	ErrSaaSSignupTenantDomainExists = errors.New("tenant domain already exists")
	ErrSaaSSignupInvalidCredentials = errors.New("invalid credentials")
	ErrSaaSSignupUserDisabled       = errors.New("user disabled")
)

type SaaSSignupService struct {
	DB         *gorm.DB
	auth       *AuthService
	keyWrapper tenantkeys.KeyWrapper
	verifier   *SignupVerificationService
}

type SaaSSignupOptions struct {
	KeyWrapper tenantkeys.KeyWrapper
	Verifier   *SignupVerificationService
}

type SaaSSignupInput struct {
	TenantKey        string
	TenantName       string
	Plan             string
	OwnerEmail       string
	OwnerPhone       string
	OwnerPassword    string
	OwnerDisplayName string
	VerificationCode string
}

type SaaSSignupResult struct {
	AccessToken  string
	RefreshToken string
	Tenant       *modeltenant.Tenant
	User         *modeliam.User
	Member       *modeliam.Member
}

func NewSaaSSignupService(db *gorm.DB, auth *AuthService, opts ...SaaSSignupOptions) *SaaSSignupService {
	var opt SaaSSignupOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	return &SaaSSignupService{DB: db, auth: auth, keyWrapper: opt.KeyWrapper, verifier: opt.Verifier}
}

func (s *SaaSSignupService) AccessTTL() time.Duration {
	if s == nil || s.auth == nil || s.auth.AccessTTL <= 0 {
		return time.Hour
	}
	return s.auth.AccessTTL
}

func (s *SaaSSignupService) Signup(ctx context.Context, in SaaSSignupInput) (*SaaSSignupResult, error) {
	if s == nil || s.DB == nil || s.auth == nil {
		return nil, fmt.Errorf("saas signup service not configured")
	}
	explicitTenantKey := strings.TrimSpace(in.TenantKey) != ""
	normalized, err := normalizeSaaSSignupInput(in)
	if err != nil {
		return nil, err
	}
	if !explicitTenantKey {
		tenantKey, err := s.ResolveTenantKey(ctx, normalized.TenantName)
		if err != nil {
			return nil, err
		}
		normalized.TenantKey = tenantKey
	}
	if s.verifier != nil {
		if err := s.verifier.Verify(ctx, chooseSaaSIdentifier(normalized.OwnerEmail, normalized.OwnerPhone), normalized.VerificationCode); err != nil {
			return nil, err
		}
	}

	var result SaaSSignupResult
	if err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txSvc := &SaaSSignupService{
			DB:         tx,
			auth:       cloneAuthServiceWithDB(s.auth, tx),
			keyWrapper: s.keyWrapper,
			verifier:   s.verifier,
		}
		created, createErr := txSvc.signupInTx(ctx, normalized)
		if createErr != nil {
			return createErr
		}
		result = *created
		return nil
	}); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *SaaSSignupService) signupInTx(ctx context.Context, in SaaSSignupInput) (*SaaSSignupResult, error) {
	tenantRepo := repotenant.NewTenantRepository(s.DB)
	userRepo := repoiam.NewUserRepository(s.DB)
	credRepo := repoiam.NewCredentialRepository(s.DB)
	memberRepo := repoiam.NewMemberRepository(s.DB)
	roleRepo := repoiam.NewRoleRepository(s.DB)
	roleBindingRepo := repoiam.NewRoleBindingRepository(s.DB)

	if existed, err := tenantRepo.GetByKey(ctx, in.TenantKey); err == nil && existed != nil {
		return nil, ErrSaaSSignupTenantKeyExists
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	domain := tenantDomainForKey(in.TenantKey)
	var domainCount int64
	if err := s.DB.WithContext(ctx).
		Model(&modeltenant.Tenant{}).
		Where("domain = ?", domain).
		Count(&domainCount).Error; err != nil {
		return nil, err
	}
	if domainCount > 0 {
		return nil, ErrSaaSSignupTenantDomainExists
	}

	identifier := chooseSaaSIdentifier(in.OwnerEmail, in.OwnerPhone)
	var user *modeliam.User
	if cred, err := credRepo.FindByProviderIdentifier(ctx, "password", identifier); err == nil && cred != nil {
		if bcrypt.CompareHashAndPassword([]byte(cred.SecretHash), []byte(in.OwnerPassword)) != nil {
			return nil, ErrSaaSSignupInvalidCredentials
		}
		loaded, loadErr := userRepo.FindByID(ctx, cred.UserID)
		if loadErr != nil {
			return nil, loadErr
		}
		if loaded.Status != modeliam.UserStatusActive {
			return nil, ErrSaaSSignupUserDisabled
		}
		user = loaded
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if user == nil {
		created, err := createSaaSOwnerUser(ctx, userRepo, credRepo, in, identifier)
		if err != nil {
			return nil, err
		}
		user = created
	}

	tenant := &modeltenant.Tenant{
		Key:    in.TenantKey,
		Name:   in.TenantName,
		Domain: domain,
		Plan:   in.Plan,
		Type:   modeltenant.TenantTypeEnterprise,
		Status: modeltenant.TenantStatusActive,
	}
	createdTenant, err := tenantRepo.Create(ctx, tenant)
	if err != nil {
		if repository.IsUniqueViolation(err, "idx_public_iam_tenant_domain") ||
			repository.IsUniqueViolation(err, "domain") {
			return nil, ErrSaaSSignupTenantDomainExists
		}
		if repository.IsUniqueViolation(err, "key") {
			return nil, ErrSaaSSignupTenantKeyExists
		}
		return nil, err
	}
	tenantUUID := createdTenant.UUID.String()

	if err := roleRepo.EnsureDefaultRoles(ctx, tenantUUID); err != nil {
		return nil, err
	}

	member := &modeliam.Member{
		TenantUUID:  tenantUUID,
		UserID:      user.ID,
		Username:    ownerUsername(in),
		DisplayName: utils.FirstNonEmpty(in.OwnerDisplayName, user.DisplayName, ownerUsername(in)),
		Status:      modeliam.UserStatusActive,
	}
	if _, err := memberRepo.Create(ctx, member); err != nil {
		return nil, err
	}
	_ = userRepo.UpdateLastTenantUUID(ctx, user.ID, tenantUUID)
	if err := roleBindingRepo.AssignRolesByCodes(ctx, tenantUUID, member.ID, string(iam.CodeRoleOwner), string(iam.CodeRoleAdmin), string(iam.CodeRoleUser)); err != nil {
		return nil, err
	}
	if _, _, err := apikeypermissions.EnsureTenantDefaultProfile(ctx, s.DB, tenantUUID, &member.ID); err != nil {
		return nil, err
	}
	keySvc := tenantkeys.NewTenantKeyService(s.DB)
	if s.keyWrapper != nil {
		keySvc = tenantkeys.NewTenantKeyServiceWithWrapper(s.DB, s.keyWrapper)
	}
	if _, err := keySvc.EnsureActiveKeyPair(ctx, "default", tenantUUID); err != nil {
		return nil, err
	}

	access, refresh, err := s.auth.issueTokensFor(ctx, createdTenant, user, member)
	if err != nil {
		return nil, err
	}
	return &SaaSSignupResult{
		AccessToken:  access,
		RefreshToken: refresh,
		Tenant:       createdTenant,
		User:         user,
		Member:       member,
	}, nil
}

func normalizeSaaSSignupInput(in SaaSSignupInput) (SaaSSignupInput, error) {
	out := SaaSSignupInput{
		TenantKey:        strings.ToLower(strings.TrimSpace(in.TenantKey)),
		TenantName:       strings.TrimSpace(in.TenantName),
		Plan:             strings.TrimSpace(in.Plan),
		OwnerEmail:       strings.ToLower(strings.TrimSpace(in.OwnerEmail)),
		OwnerPhone:       strings.TrimSpace(in.OwnerPhone),
		OwnerPassword:    strings.TrimSpace(in.OwnerPassword),
		OwnerDisplayName: strings.TrimSpace(in.OwnerDisplayName),
		VerificationCode: strings.TrimSpace(in.VerificationCode),
	}
	if out.Plan == "" {
		out.Plan = modeltenant.TenantPlanFree
	}
	if out.TenantName == "" {
		return out, fmt.Errorf("%w: tenant_name required", ErrSaaSSignupInvalidRequest)
	}
	if out.TenantKey == "" {
		out.TenantKey = utils.Slug(out.TenantName)
	}
	out.TenantKey = utils.Slug(out.TenantKey)
	if out.TenantKey == "" {
		return out, fmt.Errorf("%w: tenant_key could not be generated", ErrSaaSSignupInvalidRequest)
	}
	if out.TenantKey == modeltenant.SystemTenantKey {
		return out, fmt.Errorf("%w: system tenant key is protected", ErrSaaSSignupInvalidRequest)
	}
	if out.OwnerEmail == "" && out.OwnerPhone == "" {
		return out, fmt.Errorf("%w: owner_email or owner_phone required", ErrSaaSSignupInvalidRequest)
	}
	if len(out.OwnerPassword) < 6 {
		return out, fmt.Errorf("%w: owner_password too short", ErrSaaSSignupInvalidRequest)
	}
	return out, nil
}

func (s *SaaSSignupService) ResolveTenantKey(ctx context.Context, tenantName string) (string, error) {
	if s == nil || s.DB == nil {
		return "", fmt.Errorf("saas signup service not configured")
	}
	base := utils.Slug(strings.TrimSpace(tenantName))
	if base == "" {
		return "", fmt.Errorf("%w: tenant_name required", ErrSaaSSignupInvalidRequest)
	}
	if base == modeltenant.SystemTenantKey {
		base = base + "-tenant"
	}
	repo := repotenant.NewTenantRepository(s.DB)
	for i := 0; i < 100; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d", base, i+1)
		}
		existed, err := repo.GetByKey(ctx, candidate)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			available, err := s.tenantDomainAvailable(ctx, tenantDomainForKey(candidate))
			if err != nil {
				return "", err
			}
			if !available {
				continue
			}
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		if existed == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w: tenant_key exhausted", ErrSaaSSignupInvalidRequest)
}

func (s *SaaSSignupService) tenantDomainAvailable(ctx context.Context, domain string) (bool, error) {
	var count int64
	if err := s.DB.WithContext(ctx).
		Model(&modeltenant.Tenant{}).
		Where("domain = ?", domain).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}

func tenantDomainForKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key)) + ".tenant.powerx.local"
}

func createSaaSOwnerUser(ctx context.Context, userRepo *repoiam.UserRepository, credRepo *repoiam.CredentialRepository, in SaaSSignupInput, identifier string) (*modeliam.User, error) {
	user := &modeliam.User{
		Email:       in.OwnerEmail,
		Phone:       in.OwnerPhone,
		DisplayName: utils.FirstNonEmpty(in.OwnerDisplayName, ownerUsername(in)),
		Status:      modeliam.UserStatusActive,
	}
	if _, err := userRepo.Create(ctx, user); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.OwnerPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	if err := credRepo.Create(ctx, &modeliam.Credential{
		UserID:     user.ID,
		Provider:   "password",
		Identifier: identifier,
		SecretHash: string(hash),
		IsPrimary:  true,
	}); err != nil {
		return nil, err
	}
	return user, nil
}

func chooseSaaSIdentifier(email, phone string) string {
	if strings.TrimSpace(email) != "" {
		return strings.ToLower(strings.TrimSpace(email))
	}
	return strings.TrimSpace(phone)
}

func ownerUsername(in SaaSSignupInput) string {
	if in.OwnerEmail != "" {
		return strings.Split(in.OwnerEmail, "@")[0]
	}
	if in.OwnerPhone != "" {
		return in.OwnerPhone
	}
	return "owner"
}

func cloneAuthServiceWithDB(src *AuthService, db *gorm.DB) *AuthService {
	if src == nil {
		return nil
	}
	return NewAuthService(db, AuthOptions{
		JWTSecret:        src.JWTSecret,
		Issuer:           src.Issuer,
		Audience:         src.Audience,
		Platforms:        src.Platforms,
		AccessTTL:        src.AccessTTL,
		RefreshTTL:       src.RefreshTTL,
		DefaultTenantKey: src.DefaultTenantKey,
		DefaultEnv:       src.DefaultEnv,
		AllowedEnvs:      src.AllowedEnvs,
		Cache:            src.Cache,
	})
}

func (s *AuthService) issueTokensFor(ctx context.Context, ten *modeltenant.Tenant, u *modeliam.User, m *modeliam.Member) (string, string, error) {
	if s == nil || ten == nil || u == nil || m == nil {
		return "", "", errors.New("token subject missing")
	}
	if s.Audience == "" {
		return "", "", errors.New("audience misconfigured")
	}
	roleCodes, err := s.roleCodesForMember(ctx, ten.UUID.String(), m.ID)
	if err != nil {
		return "", "", err
	}
	claims := reqctx.CoreXClaims{
		Env:        s.DefaultEnv,
		TenantUUID: ten.UUID.String(),
		TenantID:   ten.ID,
		MemberUUID: m.UUID.String(),
		MemberID:   m.ID,
		UserUUID:   u.UUID.String(),
		UserID:     u.ID,
		Email:      strings.ToLower(strings.TrimSpace(u.Email)),
		Phone:      strings.TrimSpace(u.Phone),
		Platforms:  s.Platforms,
		IsRoot:     u.IsRoot,
		Roles:      roleCodes,
	}
	jti := uuid.NewString()
	access, err := pkgauth.GenerateAccessJWT(claims, s.Issuer, []string{s.Audience}, s.AccessTTL, s.JWTSecret)
	if err != nil {
		return "", "", err
	}
	refresh, err := pkgauth.GenerateRefreshJWT(claims, s.Issuer, []string{s.Audience}, jti, s.RefreshTTL, s.JWTSecret)
	if err != nil {
		return "", "", err
	}
	_ = s.RTRepo.Issue(ctx, &modeliam.RefreshToken{
		JTI:        jti,
		TenantUUID: ten.UUID.String(),
		MemberUUID: m.UUID.String(),
		UserUUID:   u.UUID.String(),
		ExpiresAt:  time.Now().Add(s.RefreshTTL).UnixMilli(),
	})
	if s.Cache != nil {
		ttl := 10 * time.Minute
		_ = s.Cache.Set(ctx, middleware.KUser(u.ID), utils.MustJSONBytes(u.ToLite()), ttl)
		_ = s.Cache.Set(ctx, middleware.KMember(m.ID), utils.MustJSONBytes(m.ToLite()), ttl)
		_ = s.Cache.Set(ctx, middleware.KTenant(ten.ID), utils.MustJSONBytes(ten.ToLite()), ttl)
	}
	return access, refresh, nil
}
