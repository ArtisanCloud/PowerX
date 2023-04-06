package powerx

import (
	"PowerX/internal/model"
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"time"
)

const CustomerTokenExpiredDuration = 60 * 60 * 24 * 3 * time.Second
const CustomerAccessTokenType = "Bearer"

type AuthorizationCustomerUseCase struct {
	db *gorm.DB
}

func NewAuthorizationCustomerUseCase(db *gorm.DB) *AuthorizationCustomerUseCase {
	return &AuthorizationCustomerUseCase{
		db: db,
	}
}

type CustomerJWTToken struct {
	AccessToken string `json:"AccessToken"`
	OpenID      string `json:"OpenID"`
	NickName    string `json:"NickName"`
	Exp         int64  `json:"exp"`
	jwt.RegisteredClaims
}

func (uc *AuthorizationCustomerUseCase) SignToken(mpCustomer *model.WechatMPCustomer, jwtSecret string) oauth2.Token {

	now := time.Now()
	expiresAt := now.Add(CustomerTokenExpiredDuration)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, CustomerJWTToken{
		AccessToken: "bar",
		OpenID:      mpCustomer.OpenID,
		NickName:    mpCustomer.NickName,
		Exp:         expiresAt.Unix(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "powerx",
			Subject:   mpCustomer.OpenID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	})

	signedToken, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		panic(errors.Wrap(err, "sign token failed"))
	}

	return oauth2.Token{
		AccessToken:  signedToken,
		TokenType:    CustomerAccessTokenType,
		RefreshToken: "",
		Expiry:       expiresAt,
	}
}
