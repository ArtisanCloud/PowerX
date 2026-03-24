package skills

import (
	"net/http"
	"strings"
	"time"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type createInstallTaskRequest struct {
	Provider   string `json:"provider"`
	Repo       string `json:"repo"`
	RepoURL    string `json:"repo_url"`
	Path       string `json:"path"`
	Ref        string `json:"ref"`
	Method     string `json:"method"`
	Source     string `json:"source"`
	SkillID    string `json:"skill_id"`
	Version    string `json:"version"`
	AutoImport *bool  `json:"auto_import"`
}

type listInstallTaskQuery struct {
	Status   string `form:"status"`
	Provider string `form:"provider"`
	Repo     string `form:"repo"`
	SkillID  string `form:"skill_id"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

func newInstallTaskHandler(installerSvc *skillservice.SkillInstallerService) *installTaskHandler {
	if installerSvc == nil {
		return nil
	}
	return &installTaskHandler{installerSvc: installerSvc}
}

func (h *installTaskHandler) Create(c *gin.Context) {
	var req createInstallTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	autoImport := true
	if req.AutoImport != nil {
		autoImport = *req.AutoImport
	}
	task, err := h.installerSvc.CreateTask(c.Request.Context(), skillservice.InstallTaskRequest{
		TenantUUID: strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context())),
		Provider:   strings.TrimSpace(req.Provider),
		Repo:       strings.TrimSpace(req.Repo),
		RepoURL:    strings.TrimSpace(req.RepoURL),
		Path:       strings.TrimSpace(req.Path),
		Ref:        strings.TrimSpace(req.Ref),
		Method:     strings.TrimSpace(req.Method),
		Source:     strings.TrimSpace(req.Source),
		SkillID:    strings.TrimSpace(req.SkillID),
		Version:    strings.TrimSpace(req.Version),
		Actor:      actorFromContext(c),
		AutoImport: autoImport,
	})
	if err != nil {
		respondSkillError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, mapInstallTask(task))
}

func (h *installTaskHandler) Get(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("taskId"))
	task, err := h.installerSvc.GetTask(c.Request.Context(), taskID)
	if err != nil {
		if skillservice.IsInstallTaskNotFound(err) {
			dto.ResponseError(c, http.StatusNotFound, "install task not found", err)
			return
		}
		respondSkillError(c, err)
		return
	}
	dto.ResponseSuccess(c, mapInstallTask(task))
}

func (h *installTaskHandler) List(c *gin.Context) {
	var q listInstallTaskQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid query params", err)
		return
	}
	items, total, err := h.installerSvc.ListTasks(c.Request.Context(), skillrepo.SkillInstallTaskFilter{
		Status:   splitCSV(q.Status),
		Provider: strings.TrimSpace(q.Provider),
		Repo:     strings.TrimSpace(q.Repo),
		SkillID:  strings.TrimSpace(q.SkillID),
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		respondSkillError(c, err)
		return
	}
	rows := make([]gin.H, 0, len(items))
	for i := range items {
		item := items[i]
		rows = append(rows, mapInstallTask(&item))
	}
	dto.ResponseSuccess(c, gin.H{
		"page":      maxInt(q.Page, 1),
		"page_size": normalizedPageSize(q.PageSize),
		"total":     total,
		"items":     rows,
	})
}

func mapInstallTask(task *skillmodel.SkillInstallTask) gin.H {
	if task == nil {
		return gin.H{}
	}
	return gin.H{
		"task_id":       task.TaskID,
		"tenant_uuid":   task.TenantUUID,
		"provider":      task.Provider,
		"repo":          task.Repo,
		"repo_url":      task.RepoURL,
		"path":          task.SourcePath,
		"ref":           task.Ref,
		"method":        task.Method,
		"source":        task.Source,
		"skill_id":      task.SkillID,
		"version":       task.Version,
		"install_path":  task.InstallPath,
		"status":        task.Status,
		"stdout_log":    task.StdoutLog,
		"stderr_log":    task.StderrLog,
		"error_summary": task.ErrorSummary,
		"requested_by":  task.RequestedBy,
		"started_at":    timeToRFC3339(task.StartedAt),
		"finished_at":   timeToRFC3339(task.FinishedAt),
		"created_at":    task.CreatedAt.Format(time.RFC3339),
		"updated_at":    task.UpdatedAt.Format(time.RFC3339),
	}
}

func timeToRFC3339(ts *time.Time) string {
	if ts == nil {
		return ""
	}
	return ts.UTC().Format(time.RFC3339)
}
