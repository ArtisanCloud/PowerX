package registry

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
)

type mockToolGrantVerifier struct {
	err error
}

func (m mockToolGrantVerifier) VerifyToolGrants(_ context.Context, _ string, _ []string) error {
	return m.err
}

type auditRecorder struct {
	rbacCalls []struct {
		subject  string
		resource string
		action   string
		allow    bool
	}
}

func (a *auditRecorder) LogAPI(context.Context, string, int, time.Duration)         {}
func (a *auditRecorder) LogBusPublish(context.Context, string, int)                 {}
func (a *auditRecorder) LogBusDeliver(context.Context, string, string, int, string) {}
func (a *auditRecorder) LogRBAC(_ context.Context, subject, resource, action string, allow bool) {
	a.rbacCalls = append(a.rbacCalls, struct {
		subject  string
		resource string
		action   string
		allow    bool
	}{subject: subject, resource: resource, action: action, allow: allow})
}

func TestValidatePayloadAuditsToolGrantsSuccess(t *testing.T) {
	svc := &Service{
		auditor:         &auditRecorder{},
		instrumentation: domain.NewInstrumentation(nil),
		toolGrants:      mockToolGrantVerifier{},
		systemActorLookup: func(context.Context) string {
			return "system"
		},
	}

	payload := RegistrationPayload{
		CapabilityID: "capabilities.text.translate",
		TenantID:     "tenant-corex",
		ContractRef:  "contracts/text#1.0.0",
		Adapters: []AdapterEndpoint{
			{
				AdapterID:     "adapter-primary",
				TransportType: "http",
				Endpoint:      "https://primary",
				Weight:        80,
				TimeoutMS:     2000,
			},
		},
		RoutingPolicy: RoutingPolicy{Strategy: "priority", CooldownSeconds: 60},
		ToolGrantIDs:  []string{"grant-demo"},
	}

	if err := svc.validatePayload(context.Background(), payload, false, "demo-actor"); err != nil {
		t.Fatalf("validatePayload returned error: %v", err)
	}

	recorder := svc.auditor.(*auditRecorder)
	if len(recorder.rbacCalls) != 1 {
		t.Fatalf("expected 1 RBAC audit call, got %d", len(recorder.rbacCalls))
	}
	call := recorder.rbacCalls[0]
	if !call.allow {
		t.Fatalf("expected allow=true, got false")
	}
	if call.subject != "demo-actor" {
		t.Fatalf("expected subject demo-actor, got %s", call.subject)
	}
}

func TestValidatePayloadAuditsToolGrantsFailure(t *testing.T) {
	verifierErr := errors.New("grant denied")
	recorder := &auditRecorder{}
	svc := &Service{
		auditor:         recorder,
		instrumentation: domain.NewInstrumentation(nil),
		toolGrants:      mockToolGrantVerifier{err: verifierErr},
		systemActorLookup: func(context.Context) string {
			return "system"
		},
	}

	payload := RegistrationPayload{
		CapabilityID: "capabilities.text.translate",
		TenantID:     "tenant-corex",
		ContractRef:  "contracts/text#1.0.0",
		Adapters: []AdapterEndpoint{
			{
				AdapterID:     "adapter-primary",
				TransportType: "http",
				Endpoint:      "https://primary",
				Weight:        80,
				TimeoutMS:     2000,
			},
		},
		RoutingPolicy: RoutingPolicy{Strategy: "priority", CooldownSeconds: 60},
		ToolGrantIDs:  []string{"grant-demo"},
	}

	if err := svc.validatePayload(context.Background(), payload, false, "actor-x"); !errors.Is(err, verifierErr) {
		t.Fatalf("expected verifier error, got %v", err)
	}
	if len(recorder.rbacCalls) != 1 {
		t.Fatalf("expected 1 RBAC audit call, got %d", len(recorder.rbacCalls))
	}
	if recorder.rbacCalls[0].allow {
		t.Fatalf("expected allow=false, got true")
	}
}
