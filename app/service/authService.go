package service

import (
	"crypto/rsa"
	"fmt"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/config/app"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"time"
)

type AuthService struct {
	Employee *models.Employee
}

var (
	StrPublicKeyPath  string
	StrPrivateKeyPath string
)

const InExpiredMonths = 3
const InExpiredDays = InExpiredMonths * 30

//const InExpiredSecond = InExpiredDays * 24 * 60 * 60
//const InExpiredSecond = 7200
const InExpiredSecond = 7200 * 12 * 3

/**
 ** 初始化构造函数
 */

// 模块初始化函数 import 包时被调用
func init() {

}

func SetupSSHKeyPath(ssh *app.SSHConfig) {
	StrPublicKeyPath = ssh.PublicKeyFile
	StrPrivateKeyPath = ssh.PrivateKeyFile
}

func NewAuthService(context *gin.Context) (r *AuthService) {
	r = &AuthService{
		Employee: models.NewEmployee(nil),
	}

	return r
}

func (srv *AuthService) CreateTokenForCustomer(customer *models.Customer) (string, bool) {

	claims := make(jwt.MapClaims)
	claims["AccessToken"] = "bar"
	claims["OpenID"] = customer.OpenID.String
	claims["NickName"] = customer.Name
	claims["ExternalEmployeeID"] = customer.ExternalUserID.String
	//claims["WXIndexID"] = customer.WXIndexID.String

	claims["exp"] = time.Now().Add(time.Second * time.Duration(InExpiredSecond)).Unix()
	return srv.CreateToken(claims)
}

func (srv *AuthService) CreateTokenForEmployee(employee *models.Employee) (string, bool) {

	claims := make(jwt.MapClaims)
	claims["AccessToken"] = "bar"
	claims["EmployeeUUID"] = employee.UUID
	claims["WXEmployeeID"] = employee.WXUserID.String
	claims["OpenID"] = employee.WXOpenID.String
	claims["exp"] = time.Now().Add(time.Second * time.Duration(InExpiredSecond)).Unix()

	return srv.CreateToken(claims)
}

func (srv *AuthService) CreateToken(claims jwt.MapClaims) (string, bool) {

	key, err := LoadPrivateKey()
	if err != nil {
		return "", false
	}

	//var key *rsa.PublicKey
	//key, err = jwt.ParseRSAPublicKeyFromPEM(signKey)

	//t := jwt.New(jwt.GetSigningMethod("RS256"))
	t := jwt.New(jwt.SigningMethodRS256)
	t.Claims = claims

	tokenString, err := t.SignedString(key)
	if err != nil {
		fmt.Println(err)
	}

	//fmt.Println("tokenString: " + tokenString)
	//var token *jwt.Token
	_, err = ParseTokenFromSignedTokenString(tokenString)
	if err != nil {
		fmt.Println(err)
		return "", false
	}

	return tokenString, true

}

func ParseTokenFromSignedTokenString(tokenString string) (*jwt.Token, error) {

	key, err := LoadPublicKey()
	if err != nil {
		return nil, err
	}
	parsedToken, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return key, nil
	})
	if err != nil {
		return nil, fmt.Errorf("error parsing token: %v", err)
	}

	return parsedToken, nil
}

func ParseAuthorization(authHeader string) (ptrClaims *jwt.MapClaims, err error) {

	const BEARER_SCHEMA = "Bearer "
	tokenString := authHeader[len(BEARER_SCHEMA):]
	//fmt.Printf("tokenstring:%v\r\n", tokenString)

	//privateKey, err := LoadPrivateKey()
	publicKey, err := LoadPublicKey()
	if err != nil {
		return nil, err
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		exp, ok := token.Claims.(jwt.MapClaims)["exp"].(float64)
		if !ok {
			return nil, fmt.Errorf("token does not contain expiry claim")
		}
		now := float64(time.Now().Unix())
		diff := exp - now
		//fmt.Printf("exp %f - now %f, token expired diff %f seconds \n", exp, now, diff)
		if exp < now {
			return nil, fmt.Errorf("token expired")
		}

		fmt.Printf("token will be expired in %f seconds \n", diff)

		return publicKey, nil
	})
	if err != nil {
		logger.Logger.Error("Problem with parsing", err)
	}

	Claims, ok := token.Claims.(jwt.MapClaims)
	if ok != true {
		logger.Logger.Error("Problem with claims", ok)
	}

	return &Claims, err
}

func LoadPublicKey() (*rsa.PublicKey, error) {
	publicKey, err := ioutil.ReadFile(StrPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("error reading public key file: %v\n", err)
	}

	key, err := jwt.ParseRSAPublicKeyFromPEM(publicKey)
	if err != nil {
		return nil, fmt.Errorf("error parsing RSA public key: %v\n", err)
	}

	return key, nil
}

func LoadPrivateKey() (*rsa.PrivateKey, error) {
	signKey, err := ioutil.ReadFile(StrPrivateKeyPath)
	//signKey, err := ioutil.ReadFile(strPublicKeyPath)
	if err != nil {
		fmt.Println("Error reading private key %x", err)
		return nil, err
	}

	key, err := jwt.ParseRSAPrivateKeyFromPEM(signKey)

	return key, err
}
