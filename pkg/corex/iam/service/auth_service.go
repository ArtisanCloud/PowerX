// pkg/corex/iam/service/auth_service.go
package service

import (
	"context"
	"errors"
	pkgauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
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
	UserRepo       *infraiam.UserRepository
	MemberRepo     *infraiam.MemberRepository
	CredRepo       *infraiam.CredentialRepository
	RoleRepo       *infraiam.RoleRepository
	MemberRoleRepo *infraiam.MemberRoleRepository
	RTRepo         *infraiam.RefreshTokenRepository

	// 配置
	JWTSecret  []byte
	Issuer     string        // e.g. "corex-auth"
	Audience   string        // e.g. "corex-admin"
	Platform   string        // e.g. "web" | "admin"
	AccessTTL  time.Duration // e.g. 15 * time.Minute
	RefreshTTL time.Duration // e.g. 14 * 24 * time.Hour
}

func NewAuthService(
	userRepo *infraiam.UserRepository,
	memberRepo *infraiam.MemberRepository,
	credRepo *infraiam.CredentialRepository,
	roleRepo *infraiam.RoleRepository,
	memberRoleRepo *infraiam.MemberRoleRepository,
	rtRepo *infraiam.RefreshTokenRepository,
	secret []byte,
) *AuthService {
	return &AuthService{
		UserRepo:       userRepo,
		MemberRepo:     memberRepo,
		CredRepo:       credRepo,
		RoleRepo:       roleRepo,
		MemberRoleRepo: memberRoleRepo,
		RTRepo:         rtRepo,
		JWTSecret:      secret,
		Issuer:         "corex-auth",
		Audience:       "corex-admin",
		Platform:       "web",
		AccessTTL:      15 * time.Minute,
		RefreshTTL:     14 * 24 * time.Hour,
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
		return nil, err
	}

	// 5) 绑定默认角色
	_ = s.MemberRoleRepo.AssignRolesByCodes(ctx, tenantID, m.ID, "role_user")

	return m, nil
}

func (s *AuthService) Login(ctx context.Context, tenantID uint64, identifier, password string) (access, refresh string, err error) {
	// 1) 找到凭证（全局）
	cred, err := s.CredRepo.FindByProviderIdentifier(ctx, "password", identifier)
	if err != nil || cred == nil {
		return "", "", errors.New("invalid credentials")
	}
	// 2) 校验密码
	if bcrypt.CompareHashAndPassword([]byte(cred.SecretHash), []byte(password)) != nil {
		return "", "", errors.New("invalid credentials")
	}
	// 3) 拿到全局用户
	u, err := s.UserRepo.FindByID(ctx, cred.UserID)
	if err != nil || u.Status != 1 {
		return "", "", errors.New("user disabled")
	}
	// 4) 租户内成员
	m, err := s.MemberRepo.FindByTenantAndUser(ctx, tenantID, u.ID)
	if err != nil || m.Status != 1 {
		return "", "", errors.New("member disabled or not found in tenant")
	}

	// 5) 取角色（如果需要把角色写进 JWT，可在 claims meta 里加。此处只演示获取）
	roles, _ := s.MemberRoleRepo.ListRolesByMember(ctx, tenantID, m.ID) // 下面给你这个方法的实现
	_ = roles                                                           // 如需要可拼接成 []string 存到 claims.Meta

	// 6) 签发 Access（scope=access, subject=userID）
	access, err = pkgauth.GenerateJWT(
		tenantIDToString(tenantID),
		uint64ToString(u.ID),
		s.Platform,
		s.Audience,
		"access",
		s.AccessTTL,
		s.JWTSecret,
	)
	if err != nil {
		return "", "", err
	}

	// 7) 签发 Refresh（scope=refresh + JTI）
	jti := uuid.NewString()
	refresh, err = pkgauth.GenerateJWTWithJTI(
		tenantIDToString(tenantID),
		uint64ToString(u.ID),
		s.Platform,
		s.Audience,
		"refresh",
		jti,
		s.RefreshTTL,
		s.JWTSecret,
	)
	if err != nil {
		return "", "", err
	}

	// 8) 落库 refresh JTI（按你模型绑定 user_id + tenant_id）
	rt := &model.RefreshToken{
		JTI:       jti,
		MemberID:  u.ID,
		TenantID:  tenantID,
		ExpiresAt: time.Now().Add(s.RefreshTTL).UnixMilli(),
	}
	if err := s.RTRepo.Issue(ctx, rt); err != nil {
		return "", "", err
	}

	return access, refresh, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshJWT string) (string, error) {
	claims, err := pkgauth.ParseAndValidate(refreshJWT, s.JWTSecret, s.Issuer, s.Audience)
	if err != nil {
		return "", errors.New("invalid token")
	}
	if claims.Scope != "refresh" {
		return "", errors.New("not refresh token")
	}
	userID, err := pkgauth.SubjectUint64(claims)
	if err != nil {
		return "", errors.New("bad subject")
	}
	tenantID, err := parseUint64(claims.TenantID)
	if err != nil {
		return "", errors.New("bad tenant")
	}

	// 校验 refresh 是否有效
	rt, err := s.RTRepo.GetByJTI(ctx, claims.ID)
	if err != nil || rt.MemberID != userID || rt.TenantID != tenantID || (rt.RevokedAt != nil) || time.Now().UnixMilli() > rt.ExpiresAt {
		return "", errors.New("refresh expired or revoked")
	}

	// 重新发 Access
	return pkgauth.GenerateJWT(
		claims.TenantID,
		claims.Subject,
		s.Platform,
		s.Audience,
		"access",
		s.AccessTTL,
		s.JWTSecret,
	)
}

func (s *AuthService) Logout(ctx context.Context, refreshJWT string) error {
	claims, err := pkgauth.ParseAndValidate(refreshJWT, s.JWTSecret, s.Issuer, s.Audience)
	if err != nil {
		return errors.New("invalid token")
	}
	if claims.Scope != "refresh" {
		return errors.New("not refresh token")
	}
	return s.RTRepo.RevokeByJTI(ctx, claims.ID, time.Now())
}

// helpers
func uint64ToString(v uint64) string       { return strconv.FormatUint(v, 10) }
func tenantIDToString(v uint64) string     { return strconv.FormatUint(v, 10) }
func parseUint64(s string) (uint64, error) { return strconv.ParseUint(s, 10, 64) }
