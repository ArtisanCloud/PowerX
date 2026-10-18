package backup

import (
	"mime"
	"net/http"
	"path/filepath"

	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) DownloadBackup(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	f, release, err := h.jobSvc.OpenDownload(c.Request.Context(), parseUint(c.Param("job_id")), resolveOperator(c))
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	defer release()
	info, err := f.Stat()
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(f.Name())}))
	c.Header("X-Content-Type-Options", "nosniff")
	c.DataFromReader(http.StatusOK, info.Size(), "application/postgresql-custom", f, nil)
}
