package shared

import (
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"gorm.io/gorm"
)

// Options describes common dependencies needed by Agent Model Hub services.
type Options struct {
	DB              *gorm.DB
	Cache           cache.ICache
	AuditSvc        audit.Service
	TenantKeySvc    *tenantkeys.TenantKeyService
	Instrumentation *instrumentation.Instrumentation
	Clock           func() time.Time
}

// Normalize wires default implementations for optional dependencies.
func (o *Options) Normalize() {
	if o.Clock == nil {
		o.Clock = time.Now
	}
	if o.Instrumentation == nil {
		o.Instrumentation = instrumentation.NewInstrumentation(nil, nil)
	}
}
