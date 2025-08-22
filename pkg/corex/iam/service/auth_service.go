// pkg/corex/iam/service/auth_service.go
package service

import (
	"context"
	"errors"
	pkgauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
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
	TenantRepo     *infraiam.TenantRepository
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

}

func NewAuthService(
	TenantRepo *infraiam.TenantRepository,
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
	if _, err := s.MemberRepo.FindByTenantAndUsername(ctx, tenantID, username); err == nil {
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
	var ten *model.Tenant
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
			m = &members[0]
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
	return access, refresh, nil
}

// 解析 tenantRef（支持 key/uuid；为空时回退默认租户）
func (s *AuthService) resolveTenant(ctx context.Context, ref string) (*model.Tenant, error) {
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
	claims, err := pkgauth.ParseAndValidate(refreshJWT, s.JWTSecret, s.Issuer, s.Audience)
	if err != nil {
		return "", errors.New("invalid token")
	}

	if claims.Scope != "refresh" {
		return "", errors.New("not refresh token")
	}

	// 校验 refresh JTI + 绑定
	rt, err := s.RTRepo.GetByJTI(ctx, claims.ID)
	if err != nil || rt == nil || (rt.RevokedAt != nil) || time.Now().UnixMilli() > rt.ExpiresAt {
		return "", errors.New("refresh expired or revoked")
	}
	if rt.MemberUUID != claims.MemberUUID || rt.TenantUUID != claims.TenantUUID {
		return "", errors.New("refresh token mismatch")
	}

	// 重新发 access（沿用原 claims，仅更新时间/过期时间）
	access, err := pkgauth.GenerateAccessJWT(
		*claims,  // 保留现有 claims（里有 MemberUUID/Tenant 等）
		s.Issuer, // 你初始化时注入
		[]string{s.Audience},
		s.AccessTTL,
		s.JWTSecret,
	)
	return access, err
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
