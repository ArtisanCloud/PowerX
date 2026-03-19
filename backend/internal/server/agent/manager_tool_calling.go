package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/factory/llm"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type ToolCallCandidate struct {
	FlowID       string
	AgentID      string
	Description  string
	RequiredArgs []string
	OptionalArgs []string
}

type toolCallingDecision struct {
	ToolCalls []toolCallingDecisionItem `json:"tool_calls"`
}

type toolCallingDecisionItem struct {
	Name       string                 `json:"name"`
	Args       map[string]interface{} `json:"args"`
	Confidence float64                `json:"confidence"`
	Reason     string                 `json:"reason"`
}

var toolDecisionJSONRe = regexp.MustCompile(`\{[\s\S]*\}`)

func (m *Manager) BuildToolCallCandidates(limit int) []ToolCallCandidate {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]ToolCallCandidate, 0, len(m.routesByFlow))
	for flowID, rec := range m.routesByFlow {
		item := ToolCallCandidate{
			FlowID:      strings.TrimSpace(flowID),
			AgentID:     strings.TrimSpace(rec.AgentID),
			Description: "",
		}
		if rec.Spec != nil {
			item.Description = strings.TrimSpace(rec.Spec.Name)
			if item.Description == "" && rec.Spec.Metadata != nil {
				item.Description = strings.TrimSpace(rec.Spec.Metadata.Description)
			}
			if rec.Spec.Metadata != nil && rec.Spec.Metadata.IO != nil {
				for _, in := range rec.Spec.Metadata.IO.Inputs {
					name := strings.TrimSpace(in.Name)
					if name == "" {
						continue
					}
					if in.Required {
						item.RequiredArgs = append(item.RequiredArgs, name)
					} else {
						item.OptionalArgs = append(item.OptionalArgs, name)
					}
				}
			}
		}
		if item.FlowID == "" {
			continue
		}
		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].FlowID < out[j].FlowID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// DetectTasksWithToolCalling 在 planner 决策层优先尝试 LLM tool-calling；若失败则回退到既有 DetectTasks。
func (m *Manager) DetectTasksWithToolCalling(ctx context.Context, text string, reqCfg *dto.ChatConfig) ([]flowschema.DetectedTask, error) {
	fallback := func() ([]flowschema.DetectedTask, error) {
		return m.DetectTasks(ctx, text)
	}
	if strings.TrimSpace(text) == "" {
		return fallback()
	}
	if reqCfg == nil {
		return fallback()
	}
	provider := strings.TrimSpace(reqCfg.Provider)
	model := strings.TrimSpace(reqCfg.ModelName)
	if provider == "" || model == "" {
		return fallback()
	}

	cands := m.BuildToolCallCandidates(48)
	if len(cands) == 0 {
		return fallback()
	}

	cli, err := llm.NewClient(provider)
	if err != nil {
		return fallback()
	}

	prompt := buildToolCallingPrompt(text, cands)
	content, err := cli.Invoke(ctx, &config.ModelConfig{
		Provider:     provider,
		Endpoint:     strings.TrimSpace(reqCfg.Endpoint),
		APIKey:       strings.TrimSpace(reqCfg.APIKey),
		Model:        model,
		SystemPrompt: strings.TrimSpace(reqCfg.SystemPrompt),
		Temperature:  0,
		MaxTokens:    minInt(maxInt(reqCfg.MaxTokens, 320), 1200),
	}, prompt)
	if err != nil {
		return fallback()
	}

	decision, err := parseToolCallingDecision(content)
	if err != nil {
		return fallback()
	}

	byFlow := make(map[string]flowschema.DetectedTask)
	candByName := make(map[string]ToolCallCandidate, len(cands))
	for _, c := range cands {
		candByName[strings.ToLower(c.FlowID)] = c
	}

	idSeq := 0
	for _, item := range decision.ToolCalls {
		name := strings.ToLower(strings.TrimSpace(item.Name))
		cand, ok := candByName[name]
		if !ok {
			continue
		}
		args, ok := validateToolCallingArgs(item.Args, cand)
		if !ok {
			continue
		}
		score := item.Confidence
		if score <= 0 {
			score = 0.82
		}
		if score > 1 {
			score = 1
		}
		if prev, exists := byFlow[cand.FlowID]; exists {
			if score > prev.Score {
				prev.Score = score
				prev.Reason = strings.TrimSpace(item.Reason)
				prev.Params = args
				byFlow[cand.FlowID] = prev
			}
			continue
		}
		idSeq++
		byFlow[cand.FlowID] = flowschema.DetectedTask{
			TaskID:   fmt.Sprintf("tc%d", idSeq),
			FlowID:   cand.FlowID,
			AgentID:  cand.AgentID,
			Score:    score,
			Strategy: "tool_calling",
			Reason:   strings.TrimSpace(item.Reason),
			Params:   args,
		}
	}

	if len(byFlow) == 0 {
		return fallback()
	}
	out := make([]flowschema.DetectedTask, 0, len(byFlow))
	for _, t := range byFlow {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

func parseToolCallingDecision(raw string) (*toolCallingDecision, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty llm output")
	}
	candidate := toolDecisionJSONRe.FindString(raw)
	if candidate == "" {
		candidate = raw
	}
	var out toolCallingDecision
	if err := json.Unmarshal([]byte(candidate), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func validateToolCallingArgs(raw map[string]interface{}, cand ToolCallCandidate) (map[string]interface{}, bool) {
	if raw == nil {
		raw = map[string]interface{}{}
	}
	allowed := make(map[string]struct{}, len(cand.RequiredArgs)+len(cand.OptionalArgs))
	for _, k := range cand.RequiredArgs {
		allowed[strings.ToLower(strings.TrimSpace(k))] = struct{}{}
	}
	for _, k := range cand.OptionalArgs {
		allowed[strings.ToLower(strings.TrimSpace(k))] = struct{}{}
	}

	out := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		if _, ok := allowed[key]; !ok {
			return nil, false
		}
		out[key] = v
	}

	for _, req := range cand.RequiredArgs {
		k := strings.ToLower(strings.TrimSpace(req))
		v, ok := out[k]
		if !ok || isEmptyToolArg(v) {
			return nil, false
		}
	}
	return out, true
}

func isEmptyToolArg(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	}
	return false
}

func buildToolCallingPrompt(text string, cands []ToolCallCandidate) string {
	var b strings.Builder
	b.WriteString("你是 Agent Planner 的工具调度器。请根据用户输入，从提供的工具清单中选择 0~4 个要调用的工具。\\n")
	b.WriteString("必须只输出 JSON，不要输出解释文本。\\n")
	b.WriteString(`输出格式: {"tool_calls":[{"name":"<flow_id>","args":{...},"confidence":0.0-1.0,"reason":"..."}]}` + "\n")
	b.WriteString("约束: 只能使用下方清单中的 name；args 只允许写该工具声明的参数。\\n\\n")
	b.WriteString("工具清单:\\n")
	for i, c := range cands {
		b.WriteString(fmt.Sprintf("%d) name=%s", i+1, c.FlowID))
		if strings.TrimSpace(c.Description) != "" {
			b.WriteString(fmt.Sprintf(", desc=%s", c.Description))
		}
		b.WriteString(fmt.Sprintf(", required=%v, optional=%v\\n", c.RequiredArgs, c.OptionalArgs))
	}
	b.WriteString("\\n用户输入:\\n")
	b.WriteString(text)
	return b.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
