package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTenantIDFromGin_AllSources(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// 1) request.Context()
	ctx := context.WithValue(c.Request.Context(), TenantIDKey, uint64(42))
	c.Request = c.Request.WithContext(ctx)
	if got := TenantIDFromGin(c); got == nil || *got != 42 {
		t.Fatalf("ctx.Value -> want 42, got %v", got)
	}

	// 2) gin.Context
	c.Request = c.Request.WithContext(context.Background())
	c.Set(string(TenantIDKey), uint64(43))
	if got := TenantIDFromGin(c); got == nil || *got != 43 {
		t.Fatalf("gin.Context -> want 43, got %v", got)
	}

	// 3) claims
	c.Request = c.Request.WithContext(context.WithValue(context.Background(), JWTClaimsKey, &CoreXClaims{TenantID: 44}))
	c.Set(string(TenantIDKey), nil)
	if got := TenantIDFromGin(c); got == nil || *got != 44 {
		t.Fatalf("claims -> want 44, got %v", got)
	}

	// 4) query as_tenant_id
	req2 := httptest.NewRequest(http.MethodGet, "/x?as_tenant_id=45", nil)
	c.Request = req2.WithContext(context.Background())
	if got := TenantIDFromGin(c); got == nil || *got != 45 {
		t.Fatalf("query -> want 45, got %v", got)
	}
}
