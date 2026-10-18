package agent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSessionHTTPWorkspaceIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)
	for _, ddl := range []string{
		`CREATE TABLE public.agents (id INTEGER PRIMARY KEY, uuid TEXT, env TEXT, tenant_uuid TEXT, source TEXT, owner_plugin_id TEXT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
		`CREATE TABLE public.agent_teams (id INTEGER PRIMARY KEY, tenant_uuid TEXT, parent_agent_id INTEGER, status TEXT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
		`CREATE TABLE public.agent_chat_sessions (id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT, env TEXT, tenant_uuid TEXT, agent_id INTEGER, user_id INTEGER, session_type TEXT DEFAULT 'legacy', team_id INTEGER DEFAULT 0, title TEXT, singleton BOOLEAN, ttl_days INTEGER, max_kb INTEGER, max_tokens INTEGER, summary TEXT, summary_at DATETIME, status TEXT, latest_at DATETIME, expired_at DATETIME, meta TEXT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
		`CREATE TABLE public.agent_chat_messages (id INTEGER PRIMARY KEY AUTOINCREMENT, uuid TEXT, env TEXT, tenant_uuid TEXT, session_id INTEGER, agent_id INTEGER, role TEXT, content TEXT, content_type TEXT, format TEXT, tokens INTEGER, size_bytes INTEGER, pinned BOOLEAN, is_error BOOLEAN, meta TEXT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}
	tenant := "f23491d2-b8cd-4107-a17b-301a44168aa6"
	require.NoError(t, db.Exec(`INSERT INTO public.agents(id,env,tenant_uuid) VALUES(8,'dev',?)`, tenant).Error)
	require.NoError(t, db.Exec(`INSERT INTO public.agent_teams(id,tenant_uuid,parent_agent_id,status) VALUES(3,?,8,'active'),(4,?,8,'active'),(5,?,9,'active')`, tenant, tenant, tenant).Error)
	h := &AgentSessionHandler{his: agentSvc.NewChatHistoryService(db), ag: agentSvc.NewAgentService(db)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(reqctx.WithTenantUUID(c.Request.Context(), tenant))
	})
	router.POST("/sessions", h.CreateSession)
	router.GET("/sessions", h.ListSessions)
	router.GET("/sessions/:id/messages", h.ListMessages)
	router.POST("/sessions/:id/messages", h.AppendMessage)
	chat := &AgentChatHandler{his: h.his, ag: h.ag}
	router.GET("/stream", chat.StreamSSE)
	router.POST("/sessions/:id/invoke", chat.InvokeSession)
	request := func(method, path, body string, want int) map[string]any {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, r)
		require.Equal(t, want, w.Code, w.Body.String())
		var result map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		return result
	}
	ids := []uint64{}
	for _, input := range []string{`{"agentId":8,"sessionType":"agent"}`, `{"agentId":8,"sessionType":"team","teamId":3}`, `{"agentId":8,"sessionType":"team","teamId":4}`} {
		result := request(http.MethodPost, "/sessions?env=dev", input, 200)
		ids = append(ids, uint64(result["data"].(map[string]any)["id"].(float64)))
	}
	request(http.MethodPost, "/sessions?env=dev", `{"agentId":8,"sessionType":"team","teamId":5}`, 400)
	for i, query := range []string{"session_type=agent", "session_type=team&team_id=3", "session_type=team&team_id=4"} {
		result := request(http.MethodGet, "/sessions?env=dev&agent_id=8&"+query, "", 200)
		items := result["data"].(map[string]any)["items"].([]any)
		require.Len(t, items, 1)
		require.Equal(t, float64(ids[i]), items[0].(map[string]any)["id"])
	}
	teamPath := fmt.Sprintf("/sessions/%d/messages?env=dev", ids[1])
	request(http.MethodGet, teamPath+"&session_type=agent", "", 409)
	request(http.MethodGet, teamPath+"&session_type=team&team_id=4", "", 409)
	request(http.MethodGet, teamPath+"&session_type=team&team_id=3", "", 200)
	request(http.MethodPost, teamPath, fmt.Sprintf(`{"sessionId":%d,"agentId":8,"role":"user","content":"must not persist"}`, ids[1]), 409)
	for _, suffix := range []string{"", "&regen_from_message_id=1"} {
		request(http.MethodGet, fmt.Sprintf("/stream?env=dev&session_id=%d&agent_id=8&session_type=agent&q=test%s", ids[1], suffix), "", 409)
	}
	request(http.MethodPost, fmt.Sprintf("/sessions/%d/invoke?env=dev", ids[1]), `{"message":"must not persist"}`, 409)
	var count int64
	require.NoError(t, db.Model(&dbmodel.AgentChatMessage{}).Count(&count).Error)
	require.Zero(t, count)
}
