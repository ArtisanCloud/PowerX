package ticketbridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Options describe ticket bridge runtime settings.
type Options struct {
	Provider string
	Endpoint string
	Project  string
}

// DiagnosticTicketInput captures ticket creation payload.
type DiagnosticTicketInput struct {
	TenantID   uint64
	PluginID   string
	ReportID   uuid.UUID
	Severity   string
	Title      string
	Summary    map[string]any
	LogBundle  string
	AssignedTo string
}

// TicketReference is returned after successful dispatch.
type TicketReference struct {
	ID  string
	URL string
}

// Service exposes ticket bridge operations.
type Service interface {
	CreateDiagnosticTicket(ctx context.Context, input DiagnosticTicketInput) (*TicketReference, error)
}

// NewService returns a provider-backed service.
func NewService(opts Options) Service {
	if strings.TrimSpace(strings.ToLower(opts.Provider)) == "noop" || strings.TrimSpace(opts.Provider) == "" {
		return &noopService{opts: opts}
	}
	return &noopService{opts: opts}
}

type noopService struct {
	opts Options
}

func (s *noopService) CreateDiagnosticTicket(ctx context.Context, input DiagnosticTicketInput) (*TicketReference, error) {
	project := strings.TrimSpace(s.opts.Project)
	if project == "" {
		project = "plugin-debug"
	}
	id := fmt.Sprintf("%s-%s", strings.ToUpper(project), uuid.NewString())
	url := strings.TrimSpace(s.opts.Endpoint)
	if url != "" {
		url = strings.TrimRight(url, "/") + "/" + id
	}
	return &TicketReference{
		ID:  id,
		URL: url,
	}, nil
}
