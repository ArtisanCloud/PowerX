package skills

import (
	"errors"
	"net/http"
	"strings"
	"time"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type registryHandler struct {
	registryRepo *skillrepo.SkillRegistryRepository
	importSvc    *skillservice.ImportService
}

type catalogHandler struct {
	db       *gorm.DB
	auditSvc *skillservice.AuditTraceService
}

type publishHandler struct {
	registryRepo *skillrepo.SkillRegistryRepository
	lifecycleSvc *skillservice.LifecycleService
}

type rollbackHandler struct {
	registryRepo *skillrepo.SkillRegistryRepository
	lifecycleSvc *skillservice.LifecycleService
}

type importHandler struct {
	importSvc *skillservice.ImportService
}

type marketplaceHandler struct {
	importSvc *skillservice.ImportService
}

type bindingHandler struct {
	bindingRepo *skillrepo.SkillCapabilityBindingRepository
	auditSvc    *skillservice.AuditTraceService
}

type auditHandler struct {
	traceRepo *skillrepo.SkillExecutionTraceRepository
	auditRepo *skillrepo.SkillLifecycleAuditRepository
}

type moduleDeps struct {
	db        *gorm.DB
	registry  *skillrepo.SkillRegistryRepository
	binding   *skillrepo.SkillCapabilityBindingRepository
	traceRepo *skillrepo.SkillExecutionTraceRepository
	auditRepo *skillrepo.SkillLifecycleAuditRepository
	auditSvc  *skillservice.AuditTraceService
	importSvc *skillservice.ImportService
	lifecycle *skillservice.LifecycleService
}

func newModuleDeps(db *gorm.DB) *moduleDeps {
	if db == nil {
		return nil
	}
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	return &moduleDeps{
		db:        db,
		registry:  registryRepo,
		binding:   skillrepo.NewSkillCapabilityBindingRepository(db),
		traceRepo: traceRepo,
		auditRepo: auditRepo,
		auditSvc:  auditSvc,
		importSvc: skillservice.NewImportService(registryRepo, auditSvc),
		lifecycle: skillservice.NewLifecycleService(registryRepo, auditSvc),
	}
}

func actorFromContext(c *gin.Context) string {
	ctx := c.Request.Context()
	actor := strings.TrimSpace(reqctx.GetSubject(ctx))
	if actor == "" {
		actor = strings.TrimSpace(c.GetHeader("X-Actor-ID"))
	}
	if actor == "" {
		actor = "system"
	}
	return actor
}

func mapSkillRecord(rec *skillmodel.SkillRegistryRecord) gin.H {
	if rec == nil {
		return gin.H{}
	}
	item := gin.H{
		"skill_id":            rec.SkillID,
		"version":             rec.Version,
		"source":              rec.Source,
		"status":              rec.Status,
		"bundle_uri":          rec.BundleURI,
		"checksum":            rec.Checksum,
		"signature":           rec.Signature,
		"source_url":          rec.SourceURL,
		"source_ref":          rec.SourceRef,
		"is_latest_published": rec.IsLatestPublished,
		"updated_at":          rec.UpdatedAt.Format(time.RFC3339),
	}
	return item
}

func respondSkillError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, skillrepo.ErrSkillNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		dto.ResponseError(c, http.StatusNotFound, "skill not found", err)
	case errors.Is(err, gorm.ErrInvalidData):
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
	default:
		msg := strings.ToLower(strings.TrimSpace(err.Error()))
		if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") || strings.Contains(msg, "must") || strings.Contains(msg, "cannot find skill.md") || strings.Contains(msg, "source_url") || strings.Contains(msg, "source resolver") {
			dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
			return
		}
		dto.ResponseError(c, http.StatusInternalServerError, "internal error", err)
	}
}
