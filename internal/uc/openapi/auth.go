package openapi

import (
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"time"
)

const OpenAPITokenExpiredDuration = 60 * 60 * 24 * 3 * time.Second
const OpenAPIAccessTokenType = "Bearer"

const AuthPlatformKey = "AuthPlatform"
const AuthPlatformIdKey = "PlatformId"

type OpenAPIJWTToken struct {
	AccessToken string `json:"AccessToken"`
	PlatformId  string `json:"PlatformId"`
	Name        string `json:"Name,omitempty"`
	Exp         int64  `json:"exp"`
	jwt.RegisteredClaims
}

type AuthorizationOpenAPIPlatformUseCase struct {
	db *gorm.DB
}

func NewAuthorizationOpenAPIPlatformUseCase(db *gorm.DB) *AuthorizationOpenAPIPlatformUseCase {
	return &AuthorizationOpenAPIPlatformUseCase{
		db: db,
	}
}

func (uc *AuthorizationOpenAPIPlatformUseCase) SignPlatformToken(platformKey string, jwtSecret string) oauth2.Token {
	now := time.Now()
	expiresAt := now.Add(OpenAPITokenExpiredDuration)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, OpenAPIJWTToken{
		AccessToken: "bar",
		PlatformId:  platformKey,
		Name:        platformKey,
		Exp:         expiresAt.Unix(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "powerx",
			Subject:   platformKey,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	})

	return uc.SignToken(token, jwtSecret, expiresAt)
}

func (uc *AuthorizationOpenAPIPlatformUseCase) SignToken(token *jwt.Token, jwtSecret string, expiresAt time.Time) oauth2.Token {

	signedToken, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		panic(errors.Wrap(err, "sign token failed"))
	}

	return oauth2.Token{
		AccessToken:  signedToken,
		TokenType:    OpenAPIAccessTokenType,
		RefreshToken: "",
		Expiry:       expiresAt,
	}
}
