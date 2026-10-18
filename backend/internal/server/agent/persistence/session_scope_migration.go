package persistence

import (
	"fmt"
	"strconv"
	"strings"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	modelagent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	"gorm.io/gorm"
)

// 历史记录只有每个用户回合都有完整、一致的执行归属时才自动归类。
// 没有 team_id 不等于单聊；摘要、未完成回合、跨团队或混合模式都保留 legacy。
func inferLegacySessionScope(session dbmodel.AgentChatSession, messages []dbmodel.AgentChatMessage) dbmodel.ChatSessionScope {
	legacy := dbmodel.ChatSessionScope{SessionType: dbmodel.SessionTypeLegacy}
	if session.SummaryAt != nil || strings.TrimSpace(session.Summary) != "" {
		return legacy
	}
	var inferred *dbmodel.ChatSessionScope
	var pending uint64
	turns := 0
	for _, message := range messages {
		switch message.Role {
		case "user":
			if pending != 0 {
				return legacy
			}
			pending = message.ID
		case "assistant":
			if pending == 0 {
				return legacy
			}
			state, _ := message.Meta["run_state"].(map[string]any)
			tasks, _ := state["tasks"].([]any)
			if len(tasks) == 0 {
				return legacy
			}
			var turn *dbmodel.ChatSessionScope
			for _, raw := range tasks {
				task, ok := raw.(map[string]any)
				if !ok || fmt.Sprint(task["message_id"]) != strconv.FormatUint(pending, 10) {
					return legacy
				}
				teamValue, declared := task["team_id"]
				if !declared || teamValue == nil {
					return legacy
				}
				teamText := strings.TrimSpace(fmt.Sprint(teamValue))
				scope := dbmodel.ChatSessionScope{SessionType: dbmodel.SessionTypeAgent}
				if teamText != "" && teamText != "0" {
					id, err := strconv.ParseUint(teamText, 10, 64)
					if err != nil || id == 0 {
						return legacy
					}
					scope = dbmodel.ChatSessionScope{SessionType: dbmodel.SessionTypeTeam, TeamID: id}
				} else if fmt.Sprint(task["agent_id"]) != strconv.FormatUint(session.AgentID, 10) {
					return legacy
				}
				if turn != nil && *turn != scope {
					return legacy
				}
				turn = &scope
			}
			if inferred != nil && *inferred != *turn {
				return legacy
			}
			inferred = turn
			pending = 0
			turns++
		case "tool": // 工具消息不改变回合归属。
		default:
			return legacy
		}
	}
	if pending != 0 || turns == 0 || inferred == nil {
		return legacy
	}
	return *inferred
}

func backfillAgentSessionScopes(db *gorm.DB) error {
	var sessions []dbmodel.AgentChatSession
	return db.Unscoped().Where("session_type = ?", dbmodel.SessionTypeLegacy).
		FindInBatches(&sessions, 100, func(tx *gorm.DB, _ int) error {
			for _, session := range sessions {
				var messages []dbmodel.AgentChatMessage
				if err := tx.Unscoped().Select("id", "role", "meta").
					Scopes(dbmodel.WithScope(session.Env, session.TenantUUID)).
					Where("session_id = ?", session.ID).Order("id ASC").Find(&messages).Error; err != nil {
					return err
				}
				scope := inferLegacySessionScope(session, messages)
				if scope.SessionType == dbmodel.SessionTypeLegacy {
					continue
				}
				if scope.SessionType == dbmodel.SessionTypeTeam {
					if session.TenantUUID == nil || !tx.Migrator().HasTable(&modelagent.AgentTeam{}) {
						continue
					}
					var count int64
					if err := tx.Model(&modelagent.AgentTeam{}).Where("id = ? AND tenant_uuid = ? AND parent_agent_id = ?", scope.TeamID, *session.TenantUUID, session.AgentID).Count(&count).Error; err != nil {
						return err
					}
					if count != 1 {
						continue
					}
				}
				if err := tx.Unscoped().Model(&dbmodel.AgentChatSession{}).
					Where("id = ? AND session_type = ?", session.ID, dbmodel.SessionTypeLegacy).
					Updates(map[string]any{"session_type": scope.SessionType, "team_id": scope.TeamID}).Error; err != nil {
					return err
				}
			}
			return nil
		}).Error
}
