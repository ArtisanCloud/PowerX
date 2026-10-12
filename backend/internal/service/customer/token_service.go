package customer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	pxauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CustomerTokenService makes JWT jti enforceable. Access-token validation is
// stateful against the Core session row, so a revoked token cannot be replayed.
type CustomerTokenService struct {
	db        *gorm.DB
	secret    []byte
	issuer    string
	accessTTL time.Duration
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

func NewCustomerTokenService(db *gorm.DB, secret []byte, issuer string, accessTTL time.Duration) *CustomerTokenService {
	return &CustomerTokenService{db: db, secret: secret, issuer: issuer, accessTTL: accessTTL}
}

func (s *CustomerTokenService) Issue(ctx context.Context, membership Membership) (string, error) {
	pair, err := s.IssuePair(ctx, membership)
	if err != nil {
		return "", err
	}
	return pair.AccessToken, nil
}

func (s *CustomerTokenService) IssuePair(ctx context.Context, membership Membership) (TokenPair, error) {
	return s.issuePair(ctx, membership, uuid.NewString())
}

func (s *CustomerTokenService) issuePair(ctx context.Context, membership Membership, familyUUID string) (TokenPair, error) {
	if s == nil || s.db == nil || len(s.secret) == 0 || membership.TenantUUID == "" || membership.CustomerUUID == "" || membership.MembershipUUID == "" {
		return TokenPair{}, ErrCustomerUpstreamDependency
	}
	ttl := s.accessTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	jti := uuid.NewString()
	if _, err := uuid.Parse(strings.TrimSpace(familyUUID)); err != nil {
		return TokenPair{}, ErrCustomerUpstreamDependency
	}
	now := time.Now()
	expires := now.Add(ttl)
	refreshToken, err := newRefreshToken()
	if err != nil {
		return TokenPair{}, ErrCustomerUpstreamDependency
	}
	session := &modelcustomer.Session{CustomerUUID: membership.CustomerUUID, TenantUUID: membership.TenantUUID, MembershipUUID: membership.MembershipUUID, SessionFamilyUUID: familyUUID, AccessTokenJTI: jti, RefreshTokenHash: hashCustomerRefreshToken(refreshToken), Source: "delegated", IssuedAt: now, ExpiresAt: expires}
	if err := s.db.WithContext(ctx).Create(session).Error; err != nil {
		return TokenPair{}, ErrCustomerUpstreamDependency
	}
	token, err := pxauth.GenerateCustomerAccessJWTWithJTI(membership.TenantUUID, membership.CustomerUUID, s.issuer, ttl, s.secret, jti)
	if err != nil {
		_ = s.db.WithContext(ctx).Delete(session).Error
		return TokenPair{}, ErrCustomerUpstreamDependency
	}
	return TokenPair{AccessToken: token, RefreshToken: refreshToken, ExpiresIn: int64(ttl.Seconds())}, nil
}

func (s *CustomerTokenService) RotateRefreshToken(ctx context.Context, refreshToken string) (TokenPair, error) {
	if s == nil || s.db == nil || strings.TrimSpace(refreshToken) == "" {
		return TokenPair{}, ErrCustomerUnauthorized
	}
	var session modelcustomer.Session
	err := s.db.WithContext(ctx).Where("refresh_token_hash = ?", hashCustomerRefreshToken(refreshToken)).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TokenPair{}, ErrCustomerUnauthorized
	}
	if err != nil {
		return TokenPair{}, ErrCustomerUpstreamDependency
	}
	if session.RevokedAt != nil || !session.ExpiresAt.After(time.Now()) {
		if revokeErr := s.revokeFamily(ctx, session.SessionFamilyUUID); revokeErr != nil {
			return TokenPair{}, revokeErr
		}
		return TokenPair{}, ErrCustomerUnauthorized
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&modelcustomer.Session{}).Where("uuid = ? AND revoked_at IS NULL", session.UUID).Update("revoked_at", time.Now())
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCustomerUnauthorized
		}
		return nil
	}); err != nil {
		if errors.Is(err, ErrCustomerUnauthorized) {
			return TokenPair{}, ErrCustomerUnauthorized
		}
		return TokenPair{}, ErrCustomerUpstreamDependency
	}
	return s.issuePair(ctx, Membership{TenantUUID: session.TenantUUID, CustomerUUID: session.CustomerUUID, MembershipUUID: session.MembershipUUID, Status: modelcustomer.StatusActive}, session.SessionFamilyUUID)
}

func (s *CustomerTokenService) revokeFamily(ctx context.Context, familyUUID string) error {
	if _, err := uuid.Parse(strings.TrimSpace(familyUUID)); err != nil {
		return ErrCustomerUpstreamDependency
	}
	if err := s.db.WithContext(ctx).Model(&modelcustomer.Session{}).Where("session_family_uuid = ? AND revoked_at IS NULL", familyUUID).Update("revoked_at", time.Now()).Error; err != nil {
		return ErrCustomerUpstreamDependency
	}
	return nil
}

func (s *CustomerTokenService) Validate(ctx context.Context, token string) (*pxauthClaims, error) {
	if s == nil || s.db == nil {
		return nil, ErrCustomerUpstreamDependency
	}
	claims, err := pxauth.ParseAndValidate(strings.TrimSpace(token), s.secret, s.issuer, "customer")
	if err != nil || claims == nil || claims.ID == "" || claims.CustomerUUID == "" || claims.TenantUUID == "" {
		return nil, ErrCustomerUnauthorized
	}
	var session modelcustomer.Session
	err = s.db.WithContext(ctx).Where("access_token_jti = ? AND customer_uuid = ? AND tenant_uuid = ? AND revoked_at IS NULL AND expires_at > ?", claims.ID, claims.CustomerUUID, claims.TenantUUID, time.Now()).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrCustomerUnauthorized
	}
	if err != nil {
		return nil, ErrCustomerUpstreamDependency
	}
	return &pxauthClaims{TenantUUID: claims.TenantUUID, CustomerUUID: claims.CustomerUUID, JTI: claims.ID}, nil
}

func (s *CustomerTokenService) Revoke(ctx context.Context, jti string) error {
	if s == nil || s.db == nil || strings.TrimSpace(jti) == "" {
		return ErrCustomerUpstreamDependency
	}
	result := s.db.WithContext(ctx).Model(&modelcustomer.Session{}).Where("access_token_jti = ? AND revoked_at IS NULL", strings.TrimSpace(jti)).Update("revoked_at", time.Now())
	if result.Error != nil {
		return ErrCustomerUpstreamDependency
	}
	if result.RowsAffected == 0 {
		return ErrCustomerUnauthorized
	}
	return nil
}

type pxauthClaims struct{ TenantUUID, CustomerUUID, JTI string }

func hashCustomerRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newRefreshToken() (string, error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
