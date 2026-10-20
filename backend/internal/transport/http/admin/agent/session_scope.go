package agent

import (
	"strconv"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func parseSessionScope(kind string, teamID uint64) (dbmodel.ChatSessionScope, error) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = dbmodel.SessionTypeAgent
		if teamID > 0 {
			kind = dbmodel.SessionTypeTeam
		}
	}
	scope := dbmodel.ChatSessionScope{SessionType: kind, TeamID: teamID}
	return scope, scope.Validate()
}

func querySessionScope(c *gin.Context) (dbmodel.ChatSessionScope, error) {
	var teamID uint64
	if raw := strings.TrimSpace(c.Query("team_id")); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || value == 0 {
			return dbmodel.ChatSessionScope{}, agentSvc.ErrSessionScopeMismatch
		}
		teamID = value
	}
	return parseSessionScope(c.Query("session_type"), teamID)
}

// 没有 workspace 参数的历史读取仍可按 ID 查看；显式 workspace 必须与存储归属一致。
func validateSessionQuery(c *gin.Context, session *dbmodel.AgentChatSession, writable bool) bool {
	if !writable && c.Query("session_type") == "" && c.Query("team_id") == "" {
		return true
	}
	scope, err := querySessionScope(c)
	if err == nil {
		err = agentSvc.ValidateSessionScope(session, session.AgentID, scope, writable)
	}
	if err != nil {
		dto.ResponseError(c, 409, err.Error(), nil)
		return false
	}
	return true
}

func (h *AgentSessionHandler) sessionForRequest(c *gin.Context, env string, tenantRef *string, writable bool) (*dbmodel.AgentChatSession, bool) {
	param := strings.TrimSpace(c.Param("id"))
	var session *dbmodel.AgentChatSession
	var err error
	if isDecimalID(param) {
		id, _ := strconv.ParseUint(param, 10, 64)
		session, err = h.his.FindSessionByID(c.Request.Context(), env, tenantRef, id)
	} else {
		session, err = h.his.FindSessionByUUID(c.Request.Context(), env, tenantRef, param)
	}
	if err != nil || session == nil {
		dto.ResponseError(c, 404, "agent.session_not_found", nil)
		return nil, false
	}
	return session, validateSessionQuery(c, session, writable)
}
