package skills

import (
	"encoding/json"
	"net/http"
	"strings"

	"gorm.io/datatypes"
	"gorm.io/gorm/clause"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
)

type bindCapabilityRequest struct {
	Version      string   `json:"version"`
	CapabilityID string   `json:"capability_id"`
	ToolGrants   []string `json:"tool_grants"`
}

func newBindingHandler(bindingRepo *skillrepo.SkillCapabilityBindingRepository, auditSvc *skillservice.AuditTraceService) *bindingHandler {
	if bindingRepo == nil {
		return nil
	}
	return &bindingHandler{bindingRepo: bindingRepo, auditSvc: auditSvc}
}

func (h *bindingHandler) BindCapability(c *gin.Context) {
	skillID := strings.TrimSpace(c.Param("skillId"))
	var req bindCapabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	if skillID == "" || strings.TrimSpace(req.Version) == "" || strings.TrimSpace(req.CapabilityID) == "" {
		dto.ResponseError(c, http.StatusBadRequest, "skill_id/version/capability_id are required", nil)
		return
	}
	grants, err := json.Marshal(req.ToolGrants)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid tool_grants", err)
		return
	}
	record := &skillmodel.SkillCapabilityBinding{
		SkillID:      skillID,
		Version:      strings.TrimSpace(req.Version),
		CapabilityID: strings.TrimSpace(req.CapabilityID),
		ToolGrants:   datatypes.JSON(grants),
		CreatedBy:    actorFromContext(c),
		UpdatedBy:    actorFromContext(c),
	}
	record.Normalize()
	saved, err := h.bindingRepo.Upsert(c.Request.Context(), record, []clause.Column{
		{Name: "skill_id"},
		{Name: "version"},
		{Name: "capability_id"},
	})
	if err != nil {
		respondSkillError(c, err)
		return
	}
	if h.auditSvc != nil {
		_ = h.auditSvc.RecordLifecycleAudit(c.Request.Context(), skillservice.LifecycleAuditInput{
			Action:   "bind",
			SkillID:  saved.SkillID,
			Version:  saved.Version,
			Operator: actorFromContext(c),
			Source:   "",
			Result:   "success",
			Reason:   saved.CapabilityID,
		})
	}
	dto.ResponseSuccess(c, gin.H{
		"binding_id":    saved.ID,
		"status":        saved.BindingStatus,
		"skill_id":      saved.SkillID,
		"version":       saved.Version,
		"capability_id": saved.CapabilityID,
	})
}
