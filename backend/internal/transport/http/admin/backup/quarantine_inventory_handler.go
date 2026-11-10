package backup

import (
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"strconv"
)

func inventoryInputError(c *gin.Context) {
	dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_QUARANTINE_INVENTORY_QUERY_INVALID", "BACKUP_QUARANTINE_INVENTORY_QUERY_INVALID", nil))
}
func (h *handler) QuarantineInventory(c *gin.Context) {
	o := backupops.QuarantineInventoryOptions{Page: 1, PageSize: 20, Current: "all", Result: "all", Diagnosis: "all", Sort: "time", Direction: "desc"}
	for key, values := range c.Request.URL.Query() {
		if len(values) != 1 {
			inventoryInputError(c)
			return
		}
		v := values[0]
		switch key {
		case "page", "page_size":
			n, e := strconv.Atoi(v)
			if e != nil || strconv.Itoa(n) != v {
				inventoryInputError(c)
				return
			}
			if key == "page" {
				o.Page = n
			} else {
				o.PageSize = n
			}
		case "current":
			o.Current = v
		case "result":
			o.Result = v
		case "diagnosis":
			o.Diagnosis = v
		case "q":
			o.Query = v
		case "sort":
			o.Sort = v
		case "direction":
			o.Direction = v
		default:
			inventoryInputError(c)
			return
		}
	}
	out, e := h.jobSvc.QuarantineInventory(c.Request.Context(), o)
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) QuarantineInventoryRequest(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		inventoryInputError(c)
		return
	}
	out, e := h.jobSvc.QuarantineInventoryRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) InspectQuarantineArtifact(c *gin.Context) {
	query := c.Request.URL.Query()
	values := query["expected_snapshot"]
	if len(query) != 1 || len(values) != 1 {
		inventoryInputError(c)
		return
	}
	ctx, cancel := quarantineContext(c)
	defer cancel()
	send, started := cleanupEventWriter(c)
	out, e := h.jobSvc.InspectQuarantineArtifact(ctx, c.Param("request_id"), values[0], func(v backupops.CleanupVerification) error { return send("progress", v) })
	if e != nil {
		if !started() {
			dto.RespondErrorFrom(c, backupops.ToAppError(e))
		} else if ctx.Err() == nil {
			_ = send("incomplete", gin.H{"request_id": c.Param("request_id")})
		}
		return
	}
	_ = send("complete", out)
}
