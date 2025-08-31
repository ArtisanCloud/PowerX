package agent

import (
	"encoding/json"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"strings"
)

// 内部保留的白名单字段（按需扩展）
var historyWhitelist = map[string]struct{}{
	"flow":      {},
	"id":        {},
	"role":      {},
	"content":   {},
	"step_id":   {},
	"timestamp": {},
}

// 内部需要剔除的字段前缀/完整键
var historyBlackKeys = map[string]struct{}{
	"_history": {},
	"_inputs":  {},
	"_vars":    {},
	"history":  {}, // 你旧的聚合
	"echo":     {}, // 防止把 params 再塞回去
}

// buildHistoryView：
// - 仅返回“瘦身后”的数组 []map[string]any
// - 去掉所有内部键（_history/_inputs/_vars/history/echo）
// - 仅白名单透传（flow/id/role/content/step_id/timestamp）
// - 自动按时间升序（缺失则用当前时间兜底）
// - 不做 concat，避免再拼长文本；后续若需要可以另加 join 方法
func buildHistoryView(results map[string]*agentschema.ExecutionResult, currentFlowID string) []map[string]any {
	view := make([]map[string]any, 0, len(results))
	for tid, r := range results {
		if r == nil {
			continue
		}
		entry := map[string]any{
			"task_id": tid,
			"flow":    r.Data["flow"], // 你的 Agent 会把 "flow": flowID 放在 Data 里
			"data":    map[string]any{},
		}
		// 拷贝一份 data（避免被后续修改）
		if r.Data != nil {
			cp := make(map[string]any, len(r.Data))
			for k, v := range r.Data {
				cp[k] = v
			}
			entry["data"] = cp
		}
		view = append(view, entry)
	}
	return view
}

// （可选）若你想要一个拼接字符串的视图（兼容你之前的 accum_text 行为），留个辅助：
func joinHistoryContents(results map[string]*agentschema.ExecutionResult) string {
	var sb strings.Builder
	for _, r := range results {
		if r == nil || r.Data == nil {
			continue
		}
		if s, ok := r.Data["content"].(string); ok && s != "" {
			sb.WriteString(s)
			if !strings.HasSuffix(s, "\n") {
				sb.WriteString("\n")
			}
		} else {
			b, _ := json.Marshal(r.Data)
			sb.Write(b)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
