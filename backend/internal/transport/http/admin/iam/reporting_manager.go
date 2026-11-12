package iam

import (
	"bytes"
	"encoding/json"
	svc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

func (h *MemberHandler) SetReportingManager(c *gin.Context) {
	tenant, ok := requireTenantUUIDFromContext(c)
	if !ok {
		return
	}
	var input svc.ReportingManagerInput
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 8192))
	var fields map[string]json.RawMessage
	if err != nil || json.Unmarshal(raw, &fields) != nil {
		dto.ResponseError(c, 400, "IAM_ORGANIZATION_INVALID", nil)
		return
	}
	if _, exists := fields["manager_member_uuid"]; !exists {
		dto.ResponseError(c, 400, "IAM_ORGANIZATION_INVALID", nil)
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		dto.ResponseError(c, 400, "IAM_ORGANIZATION_INVALID", nil)
		return
	}
	if err := h.S.SetReportingManager(c.Request.Context(), tenant, c.Param("id"), input); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"member_uuid": c.Param("id"), "manager_member_uuid": input.ManagerMemberUUID})
}
