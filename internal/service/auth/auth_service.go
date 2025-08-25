// pkg/corex/iam/service/auth_service.go
package auth

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/internal/service"
	pkgauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tenantmdl "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	infratenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"strconv"
	"strings"
	"time"

	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
)

type RegisterOptions struct {
	UserEmail         string // 写到 User.Email（可选）
	UserPhone         string // 写到 User.Phone（可选）
	MemberDisplayName string // 写到 Member.DisplayName（可选）
	MemberAvatarURL   string // 写到 Member.AvatarURL（可选）
}

type AuthService struct {
	*service.BaseService
	TenantRepo     *infratenant.TenantRepository
	UserRepo       *infraiam.UserRepository
	MemberRepo     *infraiam.MemberRepository
	CredRepo       *infraiam.CredentialRepository
	RoleRepo       *infraiam.RoleRepository
	MemberRoleRepo *infraiam.MemberRoleRepository
	RTRepo         *infraiam.RefreshTokenRepository

	// 配置
	JWTSecret  []byte
	Issuer     string        // e.g. "powerx-auth"
	Audience   string        // e.g. "corex-admin"
	Platforms  []string      // e.g. "web" | "admin"
	AccessTTL  time.Duration // e.g. 15 * time.Minute
	RefreshTTL time.Duration // e.g. 14 * 24 * time.Hour

	DefaultTenantKey string // e.g. "system"

	Cache cache.ICache
}

func NewAuthService(
	TenantRepo *infratenant.TenantRepository,
	userRepo *infraiam.UserRepository,
	memberRepo *infraiam.MemberRepository,
	credRepo *infraiam.CredentialRepository,
	roleRepo *infraiam.RoleRepository,
	memberRoleRepo *infraiam.MemberRoleRepository,
	rtRepo *infraiam.RefreshTokenRepository,
	secret []byte,
	issuer string, // ← cfg.Auth.Issuer
	audience string, // ← cfg.Auth.Audience
	platforms []string, // ← cfg.Auth.Platform
	accessTTL, refreshTTL time.Duration,
) *AuthService {
	return &AuthService{
		TenantRepo:       TenantRepo,
		UserRepo:         userRepo,
		MemberRepo:       memberRepo,
		CredRepo:         credRepo,
		RoleRepo:         roleRepo,
		MemberRoleRepo:   memberRoleRepo,
		RTRepo:           rtRepo,
		JWTSecret:        secret,
		Issuer:           issuer,
		Audience:         audience,
		Platforms:        platforms,
		AccessTTL:        accessTTL,
		RefreshTTL:       refreshTTL,
		DefaultTenantKey: "system", // 需要的话可改为 cfg 注入
		Cache:            cache.GetCache(),
	}
}

// Register 在 tenant 下注册一个新成员（可能复用已存在的全局 User）
func (s *AuthService) Register(ctx context.Context, tenantID uint64, username, identifier, password string, opt *RegisterOptions) (*model.Member, error) {
	if opt == nil {
		opt = &RegisterOptions{}
	}
	username = strings.ToLower(strings.TrimSpace(username))
	identifier = strings.ToLower(strings.TrimSpace(identifier))

	// 1) 租户内 username 唯一
	if _, err := s.MemberRepo.FindByTenantAndUserName(ctx, tenantID, username); err == nil {
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
		TenantID:    tenantID,
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
	_ = s.MemberRoleRepo.AssignRolesByCodes(ctx, tenantID, m.ID, "role_user")

	return m, nil
}

func (s *AuthService) Login(ctx context.Context, tenantRef, identifier, password string) (access, refresh string, err error) {
	tenantRef = strings.TrimSpace(tenantRef)
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
	if tenantRef != "" {
		ten, err = s.resolveTenant(ctx, tenantRef) // 支持 key/uuid
		if err != nil {
			return "", "", err
		}
		m, err = s.MemberRepo.FindByTenantAndUser(ctx, ten.ID, u.ID)
		if err != nil {
			return "", "", errors.New("no membership in tenant")
		}
	} else {
		// 未显式指定租户：按 membership 数量自动/报错
		members, err := s.MemberRepo.ListByUserID(ctx, u.ID) // 需要实现：按 user_id 列出成员
		if err != nil {
			return "", "", err
		}
		switch len(members) {
		case 0:
			return "", "", errors.New("no membership found")
		case 1:
			m = members[0]
			ten, err = s.TenantRepo.GetByID(ctx, m.TenantID)
			if err != nil {
				return "", "", err
			}
		default:
			return "", "", errors.New("tenant required")
		}
	}
	if m.Status != 1 {
		return "", "", errors.New("member disabled")
	}

	// 3) 用 UUID 签发（audience 必须非空）
	claims := pkgauth.CoreXClaims{
		TenantUUID: ten.UUID.String(), TenantID: ten.ID,
		MemberUUID: m.UUID.String(), MemberID: m.ID,
		UserUUID: u.UUID.String(), UserID: u.ID,
		Platforms: s.Platforms,
		IsRoot:    u.IsRoot,
	}
	//fmt.Dump(ctx, "jwt sign(access)",
	//	"issuer", s.Issuer,
	//	"aud", s.Audience,
	//	"member", claims.MemberUUID,
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
		_ = s.Cache.Set(ctx, pkgauth.KUser(u.ID), utils.MustJSONBytes(u.ToLite()), ttl)
		_ = s.Cache.Set(ctx, pkgauth.KMember(m.ID), utils.MustJSONBytes(m.ToLite()), ttl)
		_ = s.Cache.Set(ctx, pkgauth.KTenant(ten.ID), utils.MustJSONBytes(ten.ToLite()), ttl)
		// 刷新接口里再把旧 jti 写入撤销集合：_ = s.Cache.Set(ctx, kRevoked(oldJTI), []byte("1"), accessLeftTTL)
	}

	return access, refresh, nil
}

// 解析 tenantRef（支持 key/uuid；为空时回退默认租户）
func (s *AuthService) resolveTenant(ctx context.Context, ref string) (*tenantmdl.Tenant, error) {
	if ref == "" {
		if s.DefaultTenantKey == "" {
			return nil, errors.New("tenant is required")
		}
		return s.TenantRepo.EnsureByKey(ctx, s.DefaultTenantKey, "Default")
	}
	if len(ref) >= 32 && looksLikeUUID(ref) {
		return s.TenantRepo.GetByUUID(ctx, ref, nil)
	}
	return s.TenantRepo.EnsureByKey(ctx, ref, strings.Title(ref))
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
	if rt.MemberUUID != claims.MemberUUID || rt.TenantUUID != claims.TenantUUID {
		return "", errors.New("refresh token mismatch")
	}

	// 3) 从仓库加载 user / member / tenant（用于后续快照写缓存）
	//    —— 优先用 ID（claims.UserID / MemberID / TenantID）
	u, err := s.UserRepo.FindByID(ctx, claims.UserID)
	if err != nil || u == nil {
		return "", errors.New("user not found")
	}
	m, err := s.MemberRepo.GetByCondition(ctx, map[string]interface{}{
		"id = ?":        claims.MemberID,
		"tenant_id = ?": claims.TenantID, // 双重约束，防串租户
	}, nil)
	if err != nil || m == nil {
		return "", errors.New("member not found")
	}
	ten, err := s.TenantRepo.GetByID(ctx, claims.TenantID)
	if err != nil || ten == nil {
		return "", errors.New("tenant not found")
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

	// 4) 重新签发新的 access（沿用 claims 的主体信息，刷新有效期）
	access, err := pkgauth.GenerateAccessJWT(
		*claims, // Tenant/User/Member/IsRoot/Platforms 都沿用
		s.Issuer,
		[]string{s.Audience},
		s.AccessTTL,
		s.JWTSecret,
	)
	if err != nil {
		return "", err
	}

	// （可选）5) 刷新置换：如果你需要“rotate refresh”（安全性更高）
	//  - 撤销旧的 refresh jti
	//  - 生成新的 refresh 并回写 RT 表
	//  - 同时把旧 jti 放进黑名单缓存（可选，access 不用 jti 可略）
	//
	// newJTI := uuid.NewString()
	// newRefresh, err := pkgauth.GenerateRefreshJWT(*claims, s.Issuer, []string{s.Audience}, newJTI, s.RefreshTTL, s.JWTSecret)
	// if err != nil { return "", err }
	// // 撤销旧 refresh
	// if err := s.RTRepo.RevokeByJTI(ctx, claims.ID, time.Now().UnixMilli()); err != nil { return "", err }
	// // 记录新 refresh
	// _ = s.RTRepo.Issue(ctx, &model.RefreshToken{
	//     JTI:        newJTI,
	//     TenantUUID: rt.TenantUUID,
	//     MemberUUID: rt.MemberUUID,
	//     UserUUID:   rt.UserUUID,
	//     ExpiresAt:  time.Now().Add(s.RefreshTTL).UnixMilli(),
	// })
	// // （若你也对 access 做 jti 黑名单，这里可把旧 access jti 写入黑名单）
	// return access, newRefresh  // ← 如果你要同时返回新的 refresh

	// 6) 刷新缓存快照（命中率高，避免每次鉴权都查库）
	if s.Cache != nil {
		ttl := 10 * time.Minute
		// 如果你有 u.ToLite()/m.ToLite()/ten.ToLite() 直接用；否则就用下面 map 版：
		_ = s.Cache.Set(ctx, pkgauth.KUser(u.ID), utils.MustJSONBytes(map[string]any{
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
		_ = s.Cache.Set(ctx, pkgauth.KMember(m.ID), utils.MustJSONBytes(map[string]any{
			"id":         m.ID,
			"tenant_id":  m.TenantID,
			"user_id":    m.UserID,
			"status":     m.Status,
			"username":   m.Username,
			"name":       m.DisplayName,
			"avatar_url": m.AvatarURL,
			"meta":       m.Meta,
		}), ttl)
		_ = s.Cache.Set(ctx, pkgauth.KTenant(ten.ID), utils.MustJSONBytes(map[string]any{
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

// helpers
func uint64ToString(v uint64) string       { return strconv.FormatUint(v, 10) }
func tenantIDToString(v uint64) string     { return strconv.FormatUint(v, 10) }
func parseUint64(s string) (uint64, error) { return strconv.ParseUint(s, 10, 64) }
