package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
)

func TestChangeMyPasswordRequestAllowsShortCurrentPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/user/auth/me/password", bytes.NewBufferString(`{
		"current_password": "root",
		"new_password": "root111111"
	}`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	var payload changeMyPasswordReq
	if err := dto.ValidateRequestWithContext(c, &payload); err != nil {
		t.Fatalf("expected short current password to pass validation, got %v", err)
	}
}

func TestChangeMyPasswordRequestRequiresStrongNewPassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/user/auth/me/password", bytes.NewBufferString(`{
		"current_password": "root",
		"new_password": "123"
	}`))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	var payload changeMyPasswordReq
	if err := dto.ValidateRequestWithContext(c, &payload); err == nil {
		t.Fatal("expected short new password to fail validation")
	}
}
