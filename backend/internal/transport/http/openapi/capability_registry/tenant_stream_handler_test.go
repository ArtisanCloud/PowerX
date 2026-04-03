package capability_registry

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestInvokeCapabilityStreamProxySSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := io.NopCloser(strings.NewReader("event: start\ndata: {\"ok\":true}\n\nevent: done\ndata: {\"text\":\"hello\"}\n\n"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"text/event-stream; charset=utf-8"},
				},
				Body: body,
			}, nil
		}),
	}
	h := &tenantHandler{httpClient: client}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), "11111111-1111-1111-1111-111111111111")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.POST("/api/v1/tenant/invocations/stream", h.InvokeCapabilityStream)

	body := `{
		"capability_id":"com.corex.ai.llm.stream",
		"payload":{
			"method":"POST",
			"endpoint":"/api/v1/ai/llm/stream",
			"headers":{"Content-Type":"application/json"},
			"body":{"model_key":"openai/gpt-4o-mini","inputs":[{"type":"text","text":"hi"}]}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/invocations/stream?env=dev", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	req.Host = "127.0.0.1:8077"

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("unexpected content-type: %s", ct)
	}
	text := w.Body.String()
	if !strings.Contains(text, "event: start") || !strings.Contains(text, "event: done") {
		t.Fatalf("unexpected stream body: %s", text)
	}
}
