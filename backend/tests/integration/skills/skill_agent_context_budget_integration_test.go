package skillsintegration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentservice "github.com/ArtisanCloud/PowerX/internal/service/agent"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAgentChatDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS main.agent_chat_sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		uuid TEXT,
		env TEXT,
		tenant_uuid TEXT,
		agent_id INTEGER NOT NULL,
		user_id INTEGER,
		title TEXT,
		singleton BOOLEAN DEFAULT 0,
		ttl_days INTEGER DEFAULT 3,
		max_kb INTEGER DEFAULT 200,
		max_tokens INTEGER DEFAULT 3000,
		summary TEXT,
		summary_at DATETIME,
		status TEXT DEFAULT 'active',
		latest_at DATETIME,
		expired_at DATETIME,
		meta TEXT DEFAULT '{}'
	);`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS main.agent_chat_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		env TEXT,
		tenant_uuid TEXT,
		session_id INTEGER NOT NULL,
		agent_id INTEGER NOT NULL,
		role TEXT,
		content TEXT,
		content_type TEXT,
		format TEXT DEFAULT 'text',
		tokens INTEGER DEFAULT 0,
		size_bytes INTEGER DEFAULT 0,
		pinned BOOLEAN DEFAULT 0,
		is_error BOOLEAN DEFAULT 0,
		meta TEXT DEFAULT '{}'
	);`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE IF NOT EXISTS main.agent_chat_context_summaries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME,
		updated_at DATETIME,
		deleted_at DATETIME,
		uuid TEXT,
		env TEXT,
		tenant_uuid TEXT,
		session_id INTEGER NOT NULL,
		agent_id INTEGER,
		user_id INTEGER,
		summary_id TEXT,
		source_summary_id TEXT,
		schema TEXT,
		from_message_id INTEGER,
		to_message_id INTEGER,
		compressed_messages INTEGER DEFAULT 0,
		recent_messages_kept INTEGER DEFAULT 0,
		compression_policy TEXT,
		summary_json TEXT DEFAULT '{}',
		summary_text TEXT,
		checksum TEXT,
		artifact_uri TEXT,
		meta TEXT DEFAULT '{}'
	);`).Error)
	return db
}

func TestSkillAgentContextBudget_TrimForLongConversation(t *testing.T) {
	db := setupAgentChatDB(t)
	svc := agentservice.NewChatHistoryService(db)
	ctx := context.Background()
	env := "dev"
	tenant := "tenant-context-budget"

	sess, err := svc.GetOrCreateSession(ctx, env, &tenant, 1001, 1, false, &dbmodel.AgentChatSession{
		TTLDays:   3,
		MaxKB:     500,
		MaxTokens: 10000,
	})
	require.NoError(t, err)

	for i := 0; i < 36; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		content := fmt.Sprintf("第%d轮排查：%s", i+1, strings.Repeat("INC-1001 服务不可用，检查链路与依赖。", 24))
		_, err = svc.AppendMessage(ctx, env, &tenant, sess.ID, sess.AgentID, role, content, "text/plain", 120, 0, false, nil)
		require.NoError(t, err)
	}

	structuredSummary := agentservice.SessionStructuredSummary{
		Schema:      "powerx.agent.summary.v1",
		Facts:       []string{"INC-1001 在 30 分钟内发生 3 次超时"},
		Decisions:   []string{"先隔离上游依赖，后续补偿重放"},
		OpenIssues:  []string{"根因尚未完全确认"},
		Constraints: []string{"不能中断在线流量"},
	}
	bs, err := json.Marshal(structuredSummary)
	require.NoError(t, err)
	sess.Summary = string(bs)

	basePrompt := "你是运维事故助手，只能给出可执行且简洁的建议。"
	candidateSummary := strings.Repeat("incident-triage: 适用于事故分诊、影响面评估与修复建议。", 40)
	retrieval := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		retrieval = append(retrieval, fmt.Sprintf("知识片段%d：%s", i+1, strings.Repeat("近 7 天同类告警与修复记录。", 20)))
	}
	userInput := "帮我继续排查 INC-1001，并给出下一步修复顺序。"

	noTrim, err := svc.BuildContextForLLM(ctx, env, &tenant, sess, userInput, basePrompt, candidateSummary, retrieval, agentcfg.ContextOptimizerConfig{
		Enabled:                  true,
		MaxPromptTokens:          12000,
		ReservedCompletionTokens: 1200,
		RecentMessages:           32,
		RetrievalTopK:            10,
	})
	require.NoError(t, err)
	require.Greater(t, noTrim.PromptTokens, 0)

	trimmed, err := svc.BuildContextForLLM(ctx, env, &tenant, sess, userInput, basePrompt, candidateSummary, retrieval, agentcfg.ContextOptimizerConfig{
		Enabled:                  true,
		MaxPromptTokens:          700,
		ReservedCompletionTokens: 220,
		RecentMessages:           32,
		RetrievalTopK:            10,
	})
	require.NoError(t, err)
	require.True(t, trimmed.Enabled)
	require.NotEmpty(t, trimmed.TrimActions)
	require.Less(t, trimmed.PromptTokens, noTrim.PromptTokens)
	require.Greater(t, trimmed.LayerTokenSize["L3"], 0)
	require.Greater(t, trimmed.LayerTokenSize["L5"], 0)
	require.Contains(t, trimmed.SystemPrompt, "[CONTEXT-L3 RECENT]")
	require.Contains(t, trimmed.SystemPrompt, "[CONTEXT-L1 CAPABILITIES]")
}

func TestSkillAgentContextBudget_StructuredSummaryPreferred(t *testing.T) {
	db := setupAgentChatDB(t)
	svc := agentservice.NewChatHistoryService(db)
	ctx := context.Background()
	env := "dev"
	tenant := "tenant-context-summary"

	sess, err := svc.GetOrCreateSession(ctx, env, &tenant, 1002, 1, false, nil)
	require.NoError(t, err)
	sess.Summary = `{"schema":"powerx.agent.summary.v1","facts":["A"],"decisions":["B"],"open_issues":["C"],"constraints":["D"]}`

	res, err := svc.BuildContextForLLM(ctx, env, &tenant, sess, "继续", "base", "", nil, agentcfg.ContextOptimizerConfig{
		Enabled:                  true,
		MaxPromptTokens:          1024,
		ReservedCompletionTokens: 256,
		RecentMessages:           4,
		RetrievalTopK:            2,
	})
	require.NoError(t, err)
	require.True(t, res.UsedStructuredMemo)
	require.Contains(t, res.SystemPrompt, "facts: A")
	require.Contains(t, res.SystemPrompt, "decisions: B")
}

func TestSkillAgentContextBudget_RepeatedCapabilityIntroUsesBriefContext(t *testing.T) {
	db := setupAgentChatDB(t)
	svc := agentservice.NewChatHistoryService(db)
	ctx := context.Background()
	env := "dev"
	tenant := "tenant-context-repeat-intro"

	sess, err := svc.GetOrCreateSession(ctx, env, &tenant, 1004, 1, false, nil)
	require.NoError(t, err)
	_, err = svc.AppendMessage(ctx, env, &tenant, sess.ID, sess.AgentID, "assistant", "我是模板管理助手。可创建模板、查询模板、更新模板、删除模板、列表模板。", "text/plain", 0, 0, false, datatypes.JSONMap{
		"response_mode": "capability_intro",
		"response_plan": map[string]any{
			"target_capability_ids": []any{"powerxplugin.template.basic"},
		},
	})
	require.NoError(t, err)

	recentIntro, err := svc.HasRecentCapabilityIntro(ctx, env, &tenant, sess.ID, []string{"powerxplugin.template.basic"}, 12)
	require.NoError(t, err)
	require.True(t, recentIntro)

	candidateSummary := strings.Join([]string{
		"下面是当前 Agent 已绑定/可用的能力上下文。",
		"skill(1):",
		"- 模板能力",
		"  说明: 管理 PowerXPlugin 模板，包括创建、查询、更新、删除和列表等基础操作",
		"  可用动作: create, get, update, delete, list",
		"  必要参数: action",
		"  示例问法: 帮我创建一个视频模板；查询当前模板；把这个模板状态改成启用",
		"  ref: powerxplugin.template.basic",
	}, "\n")

	res, err := svc.BuildContextForLLMWithResponsePlan(ctx, env, &tenant, sess, "你能做什么？", "base", candidateSummary, nil, agentcfg.ContextOptimizerConfig{
		Enabled:                  true,
		MaxPromptTokens:          1200,
		ReservedCompletionTokens: 300,
		RecentMessages:           4,
	}, &agentservice.ResponseContextOptions{
		ResponseMode:        "capability_intro",
		TargetCapabilityIDs: []string{"powerxplugin.template.basic"},
		IncludeExamples:     true,
		RepeatFullIntro:     false,
	})
	require.NoError(t, err)
	require.Contains(t, res.SystemPrompt, "最近已经完整介绍过")
	require.Contains(t, res.SystemPrompt, "精简列出当前已绑定能力")
	require.Contains(t, res.SystemPrompt, "BOUND_SKILLS")
	require.Contains(t, res.SystemPrompt, "- 模板能力")
	require.Contains(t, res.SystemPrompt, "管理 PowerXPlugin 模板")
	require.Contains(t, res.SystemPrompt, "可用动作: create, get, update, delete, list")
	require.NotContains(t, res.SystemPrompt, "帮我创建一个视频模板")
	require.NotContains(t, res.SystemPrompt, "必要参数")
	require.NotContains(t, res.SystemPrompt, "ref:")
	require.NotContains(t, res.SystemPrompt, "可创建模板、查询模板、更新模板")
}

func TestSkillAgentContextBudget_RollingCompressionKeepsRecentWindow(t *testing.T) {
	db := setupAgentChatDB(t)
	svc := agentservice.NewChatHistoryService(db)
	ctx := context.Background()
	env := "dev"
	tenant := "tenant-context-rolling"

	sess, err := svc.GetOrCreateSession(ctx, env, &tenant, 1003, 1, false, nil)
	require.NoError(t, err)
	sess.Summary = `{"schema":"powerx.agent.summary.v1","facts":["old fact"],"decisions":["old decision"],"updated_at":"2026-06-18T00:00:00Z","from_message_id":1,"to_message_id":10,"compressed_messages":10}`

	for i := 0; i < 100; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		_, err = svc.AppendMessage(ctx, env, &tenant, sess.ID, sess.AgentID, role, fmt.Sprintf("第%03d条上下文消息：%s", i+1, strings.Repeat("发布准备与风险检查。", 10)), "text/plain", 24, 0, false, nil)
		require.NoError(t, err)
	}

	result, err := svc.RollingCompressIfNeeded(ctx, env, &tenant, sess, agentservice.RollingContextCompressionPolicy{
		RecentMessages: 20,
		MaxMessages:    500,
		DeleteCovered:  true,
	})
	require.NoError(t, err)
	require.True(t, result.Compressed)
	require.Equal(t, 80, result.CompressedMessages)
	require.Equal(t, int64(80), result.DeletedMessages)
	require.True(t, result.PreviousSummaryUsed)
	require.Contains(t, result.Summary.Facts, "old fact")
	require.Contains(t, result.Summary.Decisions, "old decision")
	require.Equal(t, 90, result.Summary.CompressedMessages)
	require.Equal(t, 20, result.Summary.RecentMessagesKept)

	remaining, err := svc.ListMessages(ctx, env, &tenant, sess.ID, 0, 200)
	require.NoError(t, err)
	require.Len(t, remaining, 20)
	require.Contains(t, remaining[0].Content, "第081条")
	require.Contains(t, remaining[19].Content, "第100条")

	refreshed, err := svc.FindSessionByID(ctx, env, &tenant, sess.ID)
	require.NoError(t, err)
	require.Contains(t, refreshed.Summary, `"schema":"powerx.agent.summary.v1"`)
	require.NotEmpty(t, refreshed.Meta["active_context_summary_id"])

	var summaryCount int64
	require.NoError(t, db.Table("agent_chat_context_summaries").Where("session_id = ?", sess.ID).Count(&summaryCount).Error)
	require.Equal(t, int64(1), summaryCount)

	var summaryRow struct {
		SummaryID          string
		FromMessageID      uint64
		ToMessageID        uint64
		CompressedMessages int
		RecentMessagesKept int
		CompressionPolicy  string
	}
	require.NoError(t, db.Table("agent_chat_context_summaries").Where("session_id = ?", sess.ID).First(&summaryRow).Error)
	require.Equal(t, refreshed.Meta["active_context_summary_id"], summaryRow.SummaryID)
	require.Equal(t, result.Summary.FromMessageID, summaryRow.FromMessageID)
	require.Equal(t, result.Summary.ToMessageID, summaryRow.ToMessageID)
	require.Equal(t, 90, summaryRow.CompressedMessages)
	require.Equal(t, 20, summaryRow.RecentMessagesKept)
	require.Equal(t, "rolling_summary_v1", summaryRow.CompressionPolicy)

	build, err := svc.BuildContextForLLM(ctx, env, &tenant, refreshed, "继续生成报告", "base", "", nil, agentcfg.ContextOptimizerConfig{
		Enabled:                  true,
		MaxPromptTokens:          4096,
		ReservedCompletionTokens: 512,
		RecentMessages:           20,
		RetrievalTopK:            2,
	})
	require.NoError(t, err)
	require.True(t, build.UsedStructuredMemo)
	require.Contains(t, build.SystemPrompt, "[CONTEXT-L2 MEMORY]")
	require.Contains(t, build.SystemPrompt, "old fact")
	require.Contains(t, build.SystemPrompt, "[CONTEXT-L3 RECENT]")
	require.Contains(t, build.SystemPrompt, "第100条")
}
