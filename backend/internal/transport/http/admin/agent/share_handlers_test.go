package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/gin-gonic/gin"
)

func TestCreateShareAcceptsUUIDRouteParam(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "uuid", Value: "123e4567-e89b-42d3-a456-426614174000"}}

	h := &ShareHandler{service: &agent_lifecycle.Service{}}
	h.CreateShare(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if msg, _ := resp["message"].(string); msg != "参数验证失败" {
		t.Fatalf("expected validation error, got message=%q", msg)
	}
}
