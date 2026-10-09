package scheduler

import (
	"strconv"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	svc "github.com/ArtisanCloud/PowerX/internal/service/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func invalidCleanup(c *gin.Context) { dto.RespondErrorFrom(c, svc.InvalidCleanupArgumentError()) }
func cleanupQuery(c *gin.Context, allowed ...string) bool {
	whitelist := map[string]bool{}
	for _, key := range allowed {
		whitelist[key] = true
	}
	for key, values := range c.Request.URL.Query() {
		if !whitelist[key] || len(values) != 1 {
			invalidCleanup(c)
			return false
		}
	}
	return true
}
func (h *Handler) DeleteJob(c *gin.Context) {
	if !cleanupQuery(c, "expected_revision") {
		return
	}
	if c.Request.ContentLength > 0 {
		invalidCleanup(c)
		return
	}
	revision, err := strconv.ParseUint(c.Query("expected_revision"), 10, 64)
	if err != nil || revision == 0 {
		invalidCleanup(c)
		return
	}
	result, err := h.svc.DeleteJob(c.Request.Context(), svc.DeleteJobInput{JobUUID: c.Param("job_id"), ExpectedRevision: revision})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}
func (h *Handler) DeleteAdminJob(c *gin.Context) {
	if cap.ServiceCredential(c.Request.Context()) {
		dto.RespondErrorFrom(c, svc.AdminUserRequiredError())
		return
	}
	h.DeleteJob(c)
}
func serviceCleanupCaller(c *gin.Context) bool {
	if !cap.ServiceCredential(c.Request.Context()) {
		dto.RespondErrorFrom(c, svc.ServiceActorRequiredError())
		return false
	}
	return true
}
func (h *Handler) ListCleanupJobs(c *gin.Context) {
	if !serviceCleanupCaller(c) || !cleanupQuery(c, "owner_type", "owner_id", "status", "page", "page_size") {
		return
	}
	page, size, valid := cleanupPagination(c)
	if !valid {
		return
	}
	items, total, err := h.svc.ListCleanupJobs(c.Request.Context(), svc.ListJobsInput{OwnerType: c.Query("owner_type"), OwnerID: c.Query("owner_id"), Status: c.Query("status"), Page: page, PageSize: size})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "pagination": gin.H{"total": total, "page": page, "page_size": size}})
}
func (h *Handler) GetCleanupJob(c *gin.Context) {
	if !serviceCleanupCaller(c) || !cleanupQuery(c, "include_deleted") {
		return
	}
	include := false
	if raw := c.Query("include_deleted"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			invalidCleanup(c)
			return
		}
		include = value
	}
	item, err := h.svc.GetCleanupJob(c.Request.Context(), c.Param("job_id"), include)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"item": item})
}
func (h *Handler) ListCleanupRuns(c *gin.Context) {
	if !serviceCleanupCaller(c) || !cleanupQuery(c, "page", "page_size") {
		return
	}
	page, size, valid := cleanupPagination(c)
	if !valid {
		return
	}
	items, total, err := h.svc.ListCleanupRuns(c.Request.Context(), svc.ListRunsInput{JobID: c.Param("job_id"), Page: page, PageSize: size})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "pagination": gin.H{"total": total, "page": page, "page_size": size}})
}
func (h *Handler) DeleteServiceJob(c *gin.Context) {
	if serviceCleanupCaller(c) {
		h.DeleteJob(c)
	}
}

func cleanupPagination(c *gin.Context) (int, int, bool) {
	page, size := 1, 50
	for _, field := range []struct {
		key   string
		value *int
		max   int
	}{{"page", &page, 0}, {"page_size", &size, 500}} {
		if raw := c.Query(field.key); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || (field.max > 0 && n > field.max) {
				invalidCleanup(c)
				return 0, 0, false
			}
			*field.value = n
		}
	}
	return page, size, true
}
