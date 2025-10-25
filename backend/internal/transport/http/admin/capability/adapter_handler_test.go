package capability

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	capb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/v1"
	capvalidator "github.com/ArtisanCloud/PowerX/internal/contract/capability"
	svc "github.com/ArtisanCloud/PowerX/internal/service/capability"
	"github.com/gin-gonic/gin"
)

type stubAdapterService struct {
	listFunc    func(ctx context.Context, tenantID uint64, capabilityKey, version string) ([]svc.TransportProfile, error)
	replaceFunc func(ctx context.Context, tenantID uint64, capabilityKey, version string, profiles []capvalidator.TransportProfile) error
	healthFunc  func(ctx context.Context, tenantID uint64, capabilityKey, version string, transport capb.TransportKind) (*svc.TransportHealthReport, error)
}

func (s *stubAdapterService) ListProfiles(ctx context.Context, tenantID uint64, capabilityKey, version string) ([]svc.TransportProfile, error) {
	if s.listFunc != nil {
		return s.listFunc(ctx, tenantID, capabilityKey, version)
	}
	return nil, nil
}

func (s *stubAdapterService) ReplaceProfiles(ctx context.Context, tenantID uint64, capabilityKey, version string, profiles []capvalidator.TransportProfile) error {
	if s.replaceFunc != nil {
		return s.replaceFunc(ctx, tenantID, capabilityKey, version, profiles)
	}
	return nil
}

func (s *stubAdapterService) HealthCheck(ctx context.Context, tenantID uint64, capabilityKey, version string, transport capb.TransportKind) (*svc.TransportHealthReport, error) {
	if s.healthFunc != nil {
		return s.healthFunc(ctx, tenantID, capabilityKey, version, transport)
	}
	return &svc.TransportHealthReport{Status: "healthy", CheckedAt: time.Now()}, nil
}

func TestAdapterHandlerListTransportProfiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubAdapterService{
		listFunc: func(ctx context.Context, tenantID uint64, capabilityKey, version string) ([]svc.TransportProfile, error) {
			return []svc.TransportProfile{{
				Transport:     "http",
				Mode:          "prefer",
				TimeoutMillis: 5000,
			}}, nil
		},
	}
	handler := &AdapterHandler{svc: stub}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodGet, "/?tenant_id=1", nil)
	c.Request = req
	c.Params = gin.Params{{Key: "capabilityKey", Value: "demo.cap"}, {Key: "version", Value: "1.0.0"}}

	handler.ListTransportProfiles(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Transports []map[string]any `json:"transports"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data.Transports) != 1 {
		t.Fatalf("expected 1 transport, got %d", len(resp.Data.Transports))
	}
	if resp.Data.Transports[0]["transport"] != "http" {
		t.Fatalf("unexpected transport: %+v", resp.Data.Transports[0])
	}
}

func TestAdapterHandlerRunHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubAdapterService{
		healthFunc: func(ctx context.Context, tenantID uint64, capabilityKey, version string, transport capb.TransportKind) (*svc.TransportHealthReport, error) {
			return &svc.TransportHealthReport{Status: "healthy", CheckedAt: time.Now()}, nil
		},
	}
	handler := &AdapterHandler{svc: stub}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodPost, "/?tenant_id=1", nil)
	c.Request = req
	c.Params = gin.Params{
		{Key: "capabilityKey", Value: "demo.cap"},
		{Key: "version", Value: "1.0.0"},
		{Key: "transport", Value: "http"},
	}

	handler.RunHealthCheck(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Data struct {
			Health map[string]any `json:"health"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Data.Health["status"] != "healthy" {
		t.Fatalf("unexpected health payload: %+v", resp.Data.Health)
	}
}
