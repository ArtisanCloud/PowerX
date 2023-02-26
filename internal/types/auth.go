package types

import "github.com/golang-jwt/jwt/v4"

type TokenClaims struct {
	UID     int64
	Account string
	Roles   []string
	*jwt.RegisteredClaims
}
