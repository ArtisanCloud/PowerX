package agent

import (
	"context"

	agentSchemas "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/cloudwego/eino/schema"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// services/agent/helper.go

func NewResultPipe(cap int) (
	*schema.StreamReader[*agentSchemas.ExecutionResult],
	*schema.StreamWriter[*agentSchemas.ExecutionResult],
) {
	return schema.Pipe[*agentSchemas.ExecutionResult](cap)
}

// ===== Debug toggles =====
const intentDebug = true            // 总开关：控制是否打印调试日志
const intentDebugAllRuleHits = true // 可选：扫描并打印“本子句命中的所有 rule 流”作为参考

// 轻量日志函数
func dlog(format string, args ...any) {
	if intentDebug {
		logger.DebugF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "agent.intent_debug"}), "[intent-debug] "+format, args...)
	}
}

// 只诊断：扫描所有路由，用和 RuleMatch 一致的打分方式找出“本子句规则命中的所有 flow”
func (m *Manager) scanAllRuleMatches(ctx context.Context, clause string) []schemas.IntentResult {
	m.mu.RLock()
	routes := make(map[string]routeRecord, len(m.routesByFlow))
	for k, v := range m.routesByFlow {
		routes[k] = v
	}
	low := m.intentLow
	m.mu.RUnlock()

	raw := clause
	rawL := strings.ToLower(raw)
	canon := func(s string) string {
		s = strings.ToLower(s)
		var b []rune
		for _, r := range s {
			if !unicode.IsSpace(r) {
				b = append(b, r)
			}
		}
		return string(b)
	}
	rawC := canon(raw)

	var hits []schemas.IntentResult
	for flowID, rec := range routes {
		spec := rec.Spec
		if spec == nil || len(spec.Matchers) == 0 {
			continue
		}
		matched := false
		score := 0.0
		reason := "no match"

		for _, mt := range spec.Matchers {
			// 只看 keyword（你已经把正则并到 keyword 的 re:.. 或 /.../ 了）
			t := strings.ToLower(strings.TrimSpace(string(mt.Type)))
			if t != "keyword" {
				continue
			}
			for _, tok := range mt.Any {
				tok = strings.TrimSpace(tok)
				if tok == "" {
					continue
				}
				if pat, ok := parseInlineRegex(tok); ok {
					if ok2, err := regexp.MatchString(pat, raw); err == nil && ok2 {
						matched = true
						base := 0.72 + 0.01*float64(mt.Priority)
						if base > score {
							score = base
							reason = "keyword:re"
						}
					}
				} else {
					if strings.Contains(rawL, strings.ToLower(tok)) {
						matched = true
						base := 0.65 + 0.01*float64(mt.Priority)
						if base > score {
							score = base
							reason = "keyword"
						}
					} else if strings.Contains(rawC, canon(tok)) {
						matched = true
						base := 0.66 + 0.01*float64(mt.Priority)
						if base > score {
							score = base
							reason = "keyword:canon"
						}
					}
				}
			}
		}

		if matched && score >= low {
			hits = append(hits, schemas.IntentResult{
				Matched:  true,
				FlowID:   flowID,
				AgentID:  rec.AgentID,
				Score:    score,
				Strategy: "rule-scan",
				Reason:   reason,
			})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	return hits
}
