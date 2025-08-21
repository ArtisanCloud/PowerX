// pkg/auth/secret.go
package auth

var jwtSecret []byte

func SetJWTSecret(b []byte) { jwtSecret = b }
func GetJWTSecret() []byte  { return jwtSecret }
