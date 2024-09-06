package customerdomain

import (
	customerdomain2 "PowerX/internal/model/crm/customerdomain"
	"PowerX/internal/model/wechat"
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"time"
)

const CustomerTokenExpiredDuration = 60 * 60 * 24 * 3 * time.Second
const CustomerAccessTokenType = "Bearer"

const AuthCustomerIdKey = "CustomerId"
const AuthCustomerOpenIdKey = "OpenId"
const AuthCustomerKey = "AuthCustomer"

type AuthorizationCustomerDomainUseCase struct {
	db *gorm.DB
}

func NewAuthorizationCustomerDomainUseCase(db *gorm.DB) *AuthorizationCustomerDomainUseCase {
	return &AuthorizationCustomerDomainUseCase{
		db: db,
	}
}

type CustomerJWTToken struct {
	AccessToken string `json:"AccessToken"`
	OpenId      string `json:"OpenId"`
	CustomerId  int64  `json:"CustomerId,omitempty"`
	NickName    string `json:"NickName,omitempty"`
	Exp         int64  `json:"exp"`
	jwt.RegisteredClaims
}

func (uc *AuthorizationCustomerDomainUseCase) SignWebToken(customer *customerdomain2.Customer, jwtSecret string) oauth2.Token {
	now := time.Now()
	expiresAt := now.Add(CustomerTokenExpiredDuration)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, CustomerJWTToken{
		AccessToken: "bar",
		NickName:    customer.Name,
		Exp:         expiresAt.Unix(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "powerx",
			Subject:   fmt.Sprintf("%d", customer.Id),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	})

	return uc.SignToken(token, jwtSecret, expiresAt)
}

func (uc *AuthorizationCustomerDomainUseCase) SignMPToken(mpCustomer *wechat.WechatMPCustomer, jwtSecret string) oauth2.Token {
	now := time.Now()
	expiresAt := now.Add(CustomerTokenExpiredDuration)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, CustomerJWTToken{
		AccessToken: "bar",
		OpenId:      mpCustomer.OpenId,
		NickName:    mpCustomer.NickName,
		Exp:         expiresAt.Unix(),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "powerx",
			Subject:   mpCustomer.OpenId,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	})

	return uc.SignToken(token, jwtSecret, expiresAt)
}

// 反函数，从 JWT 中提取指定声明信息
func GetPayloadFromToken(signedToken string) (jwt.MapClaims, error) {
	// 解析令牌，不验证签名
	token, _, err := new(jwt.Parser).ParseUnverified(signedToken, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %v", err)
	}

	// 提取 payload 数据
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("failed to extract claims from token")
	}

	return claims, nil
}

func (uc *AuthorizationCustomerDomainUseCase) SignToken(token *jwt.Token, jwtSecret string, expiresAt time.Time) oauth2.Token {

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
