// pkg/corex/iam/service/auth_service.go
package service

import (
	"context"
	"errors"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"strconv"
	"time"

	"github.com/google/uuid"

	pkgauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	UserRepo     *infraiam.UserRepository
	CredRepo     *infraiam.CredentialRepository
	RoleRepo     *infraiam.RoleRepository
	UserRoleRepo *infraiam.UserRoleRepository
	RTRepo       *infraiam.RefreshTokenRepository

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
	credRepo *infraiam.CredentialRepository,
	roleRepo *infraiam.RoleRepository,
	urRepo *infraiam.UserRoleRepository,
	rtRepo *infraiam.RefreshTokenRepository,
	secret []byte,
) *AuthService {
	return &AuthService{
		UserRepo:     userRepo,
		CredRepo:     credRepo,
		RoleRepo:     roleRepo,
		UserRoleRepo: urRepo,
		RTRepo:       rtRepo,
		JWTSecret:    secret,
		Issuer:       "corex-auth",
		Audience:     "corex-admin",
		Platform:     "web",
		AccessTTL:    15 * time.Minute,
		RefreshTTL:   14 * 24 * time.Hour,
	}
}

// Register 同前，略…

func (s *AuthService) Login(ctx context.Context, tenantID uint64, identifier, password string) (access, refresh string, err error) {
	u, cred, err := s.CredRepo.GetUserByCredential(ctx, "password", identifier)
	if err != nil {
		return "", "", errors.New("invalid credentials")
	}
	if u.TenantID != tenantID || u.Status != 1 {
		return "", "", errors.New("user disabled or tenant mismatch")
	}
	if bcrypt.CompareHashAndPassword([]byte(cred.SecretHash), []byte(password)) != nil {
		return "", "", errors.New("invalid credentials")
	}

	roles, _ := s.UserRoleRepo.ListUserRoles(ctx, tenantID, u.ID)
	roleCodes := make([]string, 0, len(roles))
	for _, r := range roles {
		roleCodes = append(roleCodes, r.Code)
	}

	// Access：scope=access, subject=userID
	access, err = pkgauth.GenerateJWT(
		// tenantID 要存为字符串
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

	// Refresh：scope=refresh + JTI
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

	// 持久化 refresh 的 JTI
	rt := &model.RefreshToken{
		JTI:       jti,
		UserID:    u.ID,
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

	// 校验 refresh 是否未撤销且未过期
	rt, err := s.RTRepo.GetByJTI(ctx, claims.ID) // claims.ID 就是 jti
	if err != nil || rt.UserID != userID || rt.TenantID != tenantID || (rt.RevokedAt != nil) || time.Now().UnixMilli() > rt.ExpiresAt {
		return "", errors.New("refresh expired or revoked")
	}

	// 重新发 Access
	access, err := pkgauth.GenerateJWT(
		claims.TenantID,
		claims.Subject,
		s.Platform,
		s.Audience,
		"access",
		s.AccessTTL,
		s.JWTSecret,
	)
	if err != nil {
		return "", err
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
	return s.RTRepo.RevokeByJTI(ctx, claims.ID, time.Now())
}

// helpers
func uint64ToString(v uint64) string       { return strconv.FormatUint(v, 10) }
func tenantIDToString(v uint64) string     { return strconv.FormatUint(v, 10) }
func parseUint64(s string) (uint64, error) { return strconv.ParseUint(s, 10, 64) }
