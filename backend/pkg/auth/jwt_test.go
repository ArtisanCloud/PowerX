//go:build ignore

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// helper：用当前 API 签发一个 access token
func issueAccessToken(t *testing.T, secret []byte, issuer string, audiences []string) string {
	claims := reqctx.CoreXClaims{
		TenantUUID: "t-test-uuid",
		TenantID:   1,
		MemberUUID: "member-uuid-123",
		MemberID:   2,
		UserUUID:   "user-uuid-456",
		UserID:     3,
		IsRoot:     true,
		Platforms:  []string{"web"},
	}
	tok, err := GenerateAccessJWT(claims, issuer, audiences, time.Hour, secret)
	assert.NoError(t, err)
	assert.NotEmpty(t, tok)
	return tok
}

func TestJwtMiddleware_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := []byte("test-secret-key")
	issuer := "powerx-auth"
	audiences := []string{"user"}
	requiredScopes := []string{"access"}

	token := issueAccessToken(t, secret, issuer, audiences)

	r := gin.New()
	r.GET("/auth/test", middleware.JwtMiddleware(secret, issuer, audiences, requiredScopes, nil), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/auth/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestJwtMiddleware_MissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := []byte("test-secret-key")
	issuer := "powerx-auth"
	audiences := []string{"user"}
	requiredScopes := []string{"access"}

	r := gin.New()
	r.GET("/auth/test", middleware.JwtMiddleware(secret, issuer, audiences, requiredScopes, nil), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/auth/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJwtMiddleware_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := []byte("test-secret-key")
	issuer := "powerx-auth"
	audiences := []string{"user"}
	requiredScopes := []string{"access"}

	r := gin.New()
	r.GET("/auth/test", middleware.JwtMiddleware(secret, issuer, audiences, requiredScopes, nil), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/auth/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestJwtMiddleware_ScopeMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	secret := []byte("test-secret-key")
	issuer := "powerx-auth"
	audiences := []string{"user"}
	requiredScopes := []string{"refresh"} // 故意要求 refresh，但签的是 access

	token := issueAccessToken(t, secret, issuer, audiences)

	r := gin.New()
	r.GET("/auth/test", middleware.JwtMiddleware(secret, issuer, audiences, requiredScopes, nil), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "success"})
	})

	req := httptest.NewRequest("GET", "/auth/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}
