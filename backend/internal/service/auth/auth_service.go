// pkg/corex/iam/service/auth_service.go
package auth

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/internal/service"
	pkgauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tenantmdl "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	infratenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/env"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"strings"
	"time"

	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

type RegisterOptions struct {
	UserEmail         string // 写到 User.Email（可选）
	UserPhone         string // 写到 User.Phone（可选）
	MemberDisplayName string // 写到 Member.DisplayName（可选）
	MemberAvatarURL   string // 写到 Member.AvatarURL（可选）
	// 追加绑定角色编码（tenant scope），例如：role_admin。
	// Register 默认总会绑定 role_user；该字段用于补充附加角色。
	RoleCodes []string
}

type AuthOptions struct {
	JWTSecret        []byte
	Issuer           string
	Audience         string
	Platforms        []string
	AccessTTL        time.Duration
	RefreshTTL       time.Duration
	DefaultTenantKey string   // 例如 "system"
	DefaultEnv       string   // 例如 "dev" | "staging" | "prod"
	AllowedEnvs      []string // 可选：此实例允许签发/切换的环境
	Cache            cache.ICache
}

type AuthService struct {
	*service.BaseService
	tenantRepo      *infratenant.TenantRepository
	UserRepo        *infraiam.UserRepository
	MemberRepo      *infraiam.MemberRepository
	CredRepo        *infraiam.CredentialRepository
	RoleRepo        *infraiam.RoleRepository
	RoleBindingRepo *infraiam.RoleBindingRepository
	RTRepo          *infraiam.RefreshTokenRepository

	// 配置
	JWTSecret  []byte
	Issuer     string        // e.g. "powerx-auth"
	Audience   string        // e.g. "corex-admin"
	Platforms  []string      // e.g. "web" | "admin"
	AccessTTL  time.Duration // e.g. 15 * time.Minute
	RefreshTTL time.Duration // e.g. 14 * 24 * time.Hour

	DefaultTenantKey string // e.g. "system"
	DefaultEnv       string
	AllowedEnvs      []string

	Cache cache.ICache
}

func NewAuthService(db *gorm.DB, opts AuthOptions) *AuthService {

	tenantRepo := infratenant.NewTenantRepository(db)
	userRepo := infraiam.NewUserRepository(db)
	memberRepo := infraiam.NewMemberRepository(db)
	credRepo := infraiam.NewCredentialRepository(db)
	roleRepo := infraiam.NewRoleRepository(db)
	RoleBindingRepo := infraiam.NewRoleBindingRepository(db)
	rtRepo := infraiam.NewRefreshTokenRepository(db)

	return &AuthService{
		BaseService:      &service.BaseService{DB: db},
		tenantRepo:       tenantRepo,
		UserRepo:         userRepo,
		MemberRepo:       memberRepo,
		CredRepo:         credRepo,
		RoleRepo:         roleRepo,
		RoleBindingRepo:  RoleBindingRepo,
		RTRepo:           rtRepo,
		JWTSecret:        opts.JWTSecret,
		Issuer:           opts.Issuer,
		Audience:         opts.Audience,
		Platforms:        opts.Platforms,
		AccessTTL:        opts.AccessTTL,
		RefreshTTL:       opts.RefreshTTL,
		DefaultTenantKey: "system", // 需要的话可改为 cfg 注入
		DefaultEnv:       env.Canonicalize(utils.FirstNonEmpty(opts.DefaultEnv, env.Dev)),
		AllowedEnvs:      opts.AllowedEnvs,
		Cache:            cache.GetCache(),
	}
}

// Register 在 tenant 下注册一个新成员（可能复用已存在的全局 User）
func (s *AuthService) Register(ctx context.Context, tenantUUID, username, identifier, password string, opt *RegisterOptions) (*model.Member, error) {
	if opt == nil {
		opt = &RegisterOptions{}
	}
	username = strings.ToLower(strings.TrimSpace(username))
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	ten, err := s.tenantByUUID(ctx, tenantUUID)
	if err != nil {
		return nil, err
	}
	tenantUUIDStr := ten.UUID.String()

	// 1) 租户内 username 唯一
	if _, err := s.MemberRepo.FindByTenantAndUserName(ctx, tenantUUIDStr, username); err == nil {
		return nil, errors.New("username exists")
	}

	var userID uint64

	// 2) 凭证是否已存在（全局唯一：provider+identifier）
	if identifier != "" {
		if cred, err := s.CredRepo.FindByProviderIdentifier(ctx, "password", identifier); err == nil && cred != nil {
			userID = cred.UserID // 复用 User
		}
	}

	// 3) 不存在则创建全局 User + Credential
	if userID == 0 {
		u := &model.User{
			DisplayName: utils.FirstNonEmpty(opt.MemberDisplayName, username),
			Status:      1,
		}
		// 可选：把 email/phone 写到 User（如果你模型加了相应字段）
		if opt.UserEmail != "" {
			u.Email = strings.ToLower(strings.TrimSpace(opt.UserEmail))
		}
		if opt.UserPhone != "" {
			u.Phone = strings.TrimSpace(opt.UserPhone)
		}
		if _, err := s.UserRepo.Create(ctx, u); err != nil {
			return nil, err
		}
		userID = u.ID

		// 写入密码凭证（如果 identifier 为空，就用 username 兜底）
		idf := identifier
		if idf == "" {
			idf = username
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		if err := s.CredRepo.Create(ctx, &model.Credential{
			UserID:     userID,
			Provider:   "password",
			Identifier: idf,
			SecretHash: string(hash),
			IsPrimary:  true,
		}); err != nil {
			return nil, err
		}
	}

	// 4) 创建租户成员（支持昵称/头像覆盖）
	m := &model.Member{
		TenantUUID:  tenantUUIDStr,
		UserID:      userID,
		Username:    username,
		DisplayName: utils.FirstNonEmpty(opt.MemberDisplayName, username),
		AvatarURL:   strings.TrimSpace(opt.MemberAvatarURL),
		Status:      1,
	}
	if _, err := s.MemberRepo.Create(ctx, m); err != nil {
		if repository.IsUniqueViolation(err, "uk_member_tenant_user") {
			return nil, errors.New("already a member of this tenant")
		}
		if repository.IsUniqueViolation(err, "uk_member_tenant_username") {
			return nil, errors.New("username already taken in this tenant")
		}
		return nil, err
	}

	// 5) 绑定默认角色
	_ = s.RoleBindingRepo.AssignRolesByCodes(ctx, tenantUUIDStr, m.ID, "role_user")
	if len(opt.RoleCodes) > 0 {
		_ = s.RoleBindingRepo.AssignRolesByCodes(ctx, tenantUUIDStr, m.ID, opt.RoleCodes...)
	}

	if _, err := tenantkeys.NewTenantKeyService(s.DB).EnsureActiveKeyPair(ctx, "default", ten.UUID.String()); err != nil {
		// 线上建议：不要阻塞注册，只记录告警即可
		// log.Warn("ensure tenant keypair failed", "tenant", ten.ID, "err", err)
		return nil, err
	}

	return m, nil
}

func (s *AuthService) Login(ctx context.Context, tenantUUID, identifier, password string) (access, refresh string, err error) {
	tenantUUID = strings.TrimSpace(tenantUUID)
	identifier = strings.ToLower(strings.TrimSpace(identifier))

	// 1) 用 identifier 原样去找凭证（邮箱/手机/用户名都行）
	u, cred, err := s.CredRepo.GetUserByCredential(ctx, "password", identifier)
	if err != nil {
		return "", "", errors.New("invalid credentials")
	}
	if bcrypt.CompareHashAndPassword([]byte(cred.SecretHash), []byte(password)) != nil {
		return "", "", errors.New("invalid credentials")
	}
	if u.Status != 1 {
		return "", "", errors.New("user disabled")
	}

	// 2) 选择租户/成员（不从 identifier 猜租户）
	var ten *tenantmdl.Tenant
	var m *model.Member
	if tenantUUID != "" {
		ten, err = s.tenantByUUID(ctx, tenantUUID)
		if err != nil {
			return "", "", err
		}
		m, err = s.MemberRepo.FindByTenantAndUser(ctx, ten.UUID.String(), u.ID)
		if err != nil {
			return "", "", errors.New("no membership in tenant")
		}
	}
	if tenantUUID == "" {
		members, err := s.MemberRepo.ListByUserID(ctx, u.ID)
		if err != nil {
			return "", "", err
		}
		m, err = chooseLoginMember(members, u.LastTenantUUID)
		if err != nil {
			return "", "", errors.New("no membership found")
		}
		if strings.TrimSpace(m.TenantUUID) == "" {
			return "", "", errors.New("member missing tenant uuid")
		}
		ten, err = s.tenantRepo.GetByUUID(ctx, m.TenantUUID)
		if err != nil {
			return "", "", err
		}
	}
	if m.Status != 1 {
		return "", "", errors.New("member disabled")
	}
	if ten.Status != tenantmdl.TenantStatusActive {
		return "", "", errors.New("tenant disabled")
	}
	_ = s.UserRepo.UpdateLastTenantUUID(ctx, u.ID, ten.UUID.String())
	roleCodes, err := s.roleCodesForMember(ctx, ten.UUID.String(), m.ID)
	if err != nil {
		return "", "", err
	}

	// 3) 用 UUID 签发（audience 必须非空）
	claims := reqctx.CoreXClaims{
		Env:        s.DefaultEnv,
		TenantUUID: ten.UUID.String(), TenantID: ten.ID,
		MemberUUID: m.UUID.String(), MemberID: m.ID,
		UserUUID: u.UUID.String(), UserID: u.ID,
		Email:     strings.ToLower(strings.TrimSpace(u.Email)),
		Phone:     strings.TrimSpace(u.Phone),
		Platforms: s.Platforms,
		IsRoot:    u.IsRoot,
		Roles:     roleCodes,
	}
	//fmtx.Dump(ctx, "jwt sign(access)",
	//	"issuer", s.Issuer,
	//	"aud", s.Audience,
	//	"member", claims.MemberUUID,
	//	"env", claims.Env,
	//	"tenant", claims.TenantUUID,
	//	"platform", claims.Platforms,
	//	"ttl", s.AccessTTL.String(),
	//	"secret_fp", utils.SecretFP(s.JWTSecret),
	//)
	if s.Audience == "" {
		return "", "", errors.New("audience misconfigured")
	}

	jti := uuid.NewString()
	access, err = pkgauth.GenerateAccessJWT(claims, s.Issuer, []string{s.Audience}, s.AccessTTL, s.JWTSecret)
	if err != nil {
		return "", "", err
	}
	//fmt.Dump(ctx, "jwt sign(refresh)",
	//	"issuer", s.Issuer,
	//	"aud", s.Audience,
	//	"member", claims.MemberUUID,
	//	"tenant", claims.TenantUUID,
	//	"ttl", s.RefreshTTL.String(),
	//	"secret_fp", utils.SecretFP(s.JWTSecret),
	//)
	refresh, err = pkgauth.GenerateRefreshJWT(claims, s.Issuer, []string{s.Audience}, jti, s.RefreshTTL, s.JWTSecret)
	if err != nil {
		return "", "", err
	}

	_ = s.RTRepo.Issue(ctx, &model.RefreshToken{
		JTI:        jti,
		TenantUUID: ten.UUID.String(),
		MemberUUID: m.UUID.String(),
		UserUUID:   u.UUID.String(),
		ExpiresAt:  time.Now().Add(s.RefreshTTL).UnixMilli(),
	})

	if s.Cache != nil {
		ttl := 10 * time.Minute // 建议 5~15 分钟
		_ = s.Cache.Set(ctx, middleware.KUser(u.ID), utils.MustJSONBytes(u.ToLite()), ttl)
		_ = s.Cache.Set(ctx, middleware.KMember(m.ID), utils.MustJSONBytes(m.ToLite()), ttl)
		_ = s.Cache.Set(ctx, middleware.KTenant(ten.ID), utils.MustJSONBytes(ten.ToLite()), ttl)
	}

	return access, refresh, nil
}

func chooseLoginMember(members []*model.Member, preferredTenantUUID string) (*model.Member, error) {
	preferredTenantUUID = strings.TrimSpace(preferredTenantUUID)
	var firstActive *model.Member
	for _, member := range members {
		if member == nil || member.Status != model.UserStatusActive {
			continue
		}
		if firstActive == nil {
			firstActive = member
		}
		if preferredTenantUUID != "" && strings.EqualFold(strings.TrimSpace(member.TenantUUID), preferredTenantUUID) {
			return member, nil
		}
	}
	if firstActive == nil {
		return nil, errors.New("no active membership found")
	}
	return firstActive, nil
}

type TenantSwitchTokenResult struct {
	AccessToken  string
	RefreshToken string
	TenantID     uint64
	TenantUUID   string
	MemberID     uint64
	MemberUUID   string
}

func (s *AuthService) IssueTokensForTenantSwitch(ctx context.Context, userID uint64, tenantUUID string) (*TenantSwitchTokenResult, error) {
	if s == nil {
		return nil, errors.New("auth service not configured")
	}
	ten, err := s.tenantByUUID(ctx, tenantUUID)
	if err != nil {
		return nil, err
	}
	u, err := s.UserRepo.FindByID(ctx, userID)
	if err != nil || u == nil {
		return nil, errors.New("user not found")
	}
	if u.Status != model.UserStatusActive {
		return nil, errors.New("user disabled")
	}
	m, err := s.MemberRepo.FindByTenantAndUser(ctx, ten.UUID.String(), u.ID)
	if err != nil {
		return nil, errors.New("no membership in tenant")
	}
	if m.Status != model.UserStatusActive {
		return nil, errors.New("member disabled")
	}
	if ten.Status != tenantmdl.TenantStatusActive {
		return nil, errors.New("tenant disabled")
	}
	_ = s.UserRepo.UpdateLastTenantUUID(ctx, u.ID, ten.UUID.String())
	access, refresh, err := s.issueTokensFor(ctx, ten, u, m)
	if err != nil {
		return nil, err
	}
	return &TenantSwitchTokenResult{
		AccessToken:  access,
		RefreshToken: refresh,
		TenantID:     ten.ID,
		TenantUUID:   ten.UUID.String(),
		MemberID:     m.ID,
		MemberUUID:   m.UUID.String(),
	}, nil
}

// tenantByUUID 解析并加载租户记录（仅接受 UUID）
func (s *AuthService) tenantByUUID(ctx context.Context, tenantUUID string) (*tenantmdl.Tenant, error) {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return nil, errors.New("tenant uuid required")
	}
	if !looksLikeUUID(tenantUUID) {
		return nil, errors.New("tenant uuid invalid")
	}
	ten, err := s.tenantRepo.GetByUUID(ctx, tenantUUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("tenant not found")
		}
		return nil, err
	}
	if ten == nil || ten.ID == 0 {
		return nil, errors.New("tenant not found")
	}
	return ten, nil
}

func (s *AuthService) roleCodesForMember(ctx context.Context, tenantUUID string, memberID uint64) ([]string, error) {
	if s == nil || s.RoleBindingRepo == nil {
		return nil, errors.New("role binding repository not configured")
	}
	roles, err := s.RoleBindingRepo.ListRolesByMember(ctx, tenantUUID, memberID)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(roles))
	seen := map[string]struct{}{}
	for _, role := range roles {
		code := strings.ToLower(strings.TrimSpace(string(role.Code)))
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out, nil
}

func looksLikeUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshJWT string) (string, error) {
	// 1) 解析并校验 refresh token（iss/aud/exp/nbf/iat/签名）
	claims, err := pkgauth.ParseAndValidate(refreshJWT, s.JWTSecret, s.Issuer, s.Audience)
	if err != nil {
		return "", errors.New("invalid token")
	}
	if claims.Scope != "refresh" {
		return "", errors.New("not refresh token")
	}

	// 2) 校验 refresh 的 JTI 是否有效、未撤销、未过期、绑定一致
	rt, err := s.RTRepo.GetByJTI(ctx, claims.ID)
	if err != nil || rt == nil || (rt.RevokedAt != nil) || time.Now().UnixMilli() > rt.ExpiresAt {
		return "", errors.New("refresh expired or revoked")
	}
	if rt.MemberUUID != claims.MemberUUID || rt.TenantUUID != claims.TenantUUID || rt.UserUUID != claims.UserUUID {
		return "", errors.New("refresh token mismatch")
	}

	// 3) 从仓库加载 user / member / tenant（用于后续快照写缓存）
	ten, err := s.tenantByUUID(ctx, claims.TenantUUID)
	if err != nil {
		return "", err
	}
	u, err := s.UserRepo.FindByID(ctx, claims.UserID)
	if err != nil || u == nil {
		return "", errors.New("user not found")
	}
	m, err := s.MemberRepo.GetByCondition(ctx, map[string]interface{}{
		"id = ?":          claims.MemberID,
		"tenant_uuid = ?": ten.UUID.String(), // 双重约束，防串租户
	}, nil)
	if err != nil || m == nil {
		return "", errors.New("member not found")
	}
	if u.Status != 1 {
		return "", errors.New("user disabled")
	}
	if m.Status != 1 {
		return "", errors.New("member disabled")
	}
	if ten.Status != 1 {
		return "", errors.New("tenant disabled")
	}
	if strings.TrimSpace(m.UUID.String()) != strings.TrimSpace(claims.MemberUUID) ||
		strings.TrimSpace(u.UUID.String()) != strings.TrimSpace(claims.UserUUID) {
		return "", errors.New("refresh token mismatch")
	}
	roleCodes, err := s.roleCodesForMember(ctx, ten.UUID.String(), m.ID)
	if err != nil {
		return "", err
	}

	// 4) 重新签发新的 access。角色必须从数据库重新加载，不能复用 refresh token 里的旧角色快照。
	claims.TenantID = ten.ID
	claims.MemberID = m.ID
	claims.MemberUUID = m.UUID.String()
	claims.UserID = u.ID
	claims.UserUUID = u.UUID.String()
	claims.Email = strings.ToLower(strings.TrimSpace(u.Email))
	claims.Phone = strings.TrimSpace(u.Phone)
	claims.IsRoot = u.IsRoot
	claims.Roles = roleCodes
	access, err := pkgauth.GenerateAccessJWT(
		*claims, // Tenant/User/Member/Platforms/Env 都沿用
		s.Issuer,
		[]string{s.Audience},
		s.AccessTTL,
		s.JWTSecret,
	)
	if err != nil {
		return "", err
	}

	// 6) 刷新缓存快照（命中率高，避免每次鉴权都查库）
	if s.Cache != nil {
		ttl := 10 * time.Minute
		_ = s.Cache.Set(ctx, middleware.KUser(u.ID), utils.MustJSONBytes(map[string]any{
			"id":            u.ID,
			"status":        u.Status,
			"is_root":       u.IsRoot,
			"display_name":  u.DisplayName,
			"avatar_url":    u.AvatarURL,
			"email":         u.Email,
			"phone":         u.Phone,
			"last_login_at": u.LastLoginAt,
			"meta":          u.Meta,
		}), ttl)
		_ = s.Cache.Set(ctx, middleware.KMember(m.ID), utils.MustJSONBytes(map[string]any{
			"id":          m.ID,
			"tenant_uuid": m.TenantUUID,
			"user_id":     m.UserID,
			"status":      m.Status,
			"username":    m.Username,
			"name":        m.DisplayName,
			"avatar_url":  m.AvatarURL,
			"meta":        m.Meta,
		}), ttl)
		_ = s.Cache.Set(ctx, middleware.KTenant(ten.ID), utils.MustJSONBytes(map[string]any{
			"id":     ten.ID,
			"key":    ten.Key,
			"name":   ten.Name,
			"status": ten.Status,
			"plan":   ten.Plan,
			"domain": ten.Domain,
		}), ttl)
	}

	return access, nil
}

func (s *AuthService) Logout(ctx context.Context, refreshJWT string) error {
	claims, err := pkgauth.ParseAndValidate(refreshJWT, s.JWTSecret, s.Issuer, s.Audience)
	if err != nil {
		return errors.New("invalid token")
	}
	if claims.Scope != "refresh" {
		return errors.New("not refresh token")
	}
	return s.RTRepo.RevokeByJTI(ctx, claims.ID, time.Now().UnixMilli())
}
