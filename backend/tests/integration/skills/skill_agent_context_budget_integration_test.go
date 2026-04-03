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
