package skills

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type incidentTriageExecutor struct{}

var incidentIDPattern = regexp.MustCompile(`(?i)\bINC-\d+\b`)

func newIncidentTriageExecutor() SkillExecutor { return &incidentTriageExecutor{} }

func (e *incidentTriageExecutor) CanHandle(in ExecuteInput) bool {
	id := strings.ToLower(strings.TrimSpace(in.SkillID))
	return id == "incident-triage" || strings.HasSuffix(id, ".incident-triage")
}

func (e *incidentTriageExecutor) Execute(ctx context.Context, in ExecuteInput) (map[string]interface{}, error) {
	incidentID := strings.TrimSpace(extractString(in.Payload, "incident_id", "incidentId"))
	contextText := strings.TrimSpace(extractString(in.Payload, "context", "incident_context"))
	if contextText == "" {
		contextText = strings.TrimSpace(extractString(in.Context, "message", "query", "prompt"))
	}
	if incidentID == "" && contextText != "" {
		if m := incidentIDPattern.FindString(contextText); m != "" {
			incidentID = strings.ToUpper(strings.TrimSpace(m))
		}
	}
	if incidentID == "" {
		incidentID = "未提供"
	}

	severity := detectSeverity(contextText)
	actions := buildActions(severity, contextText)
	summary := buildSummary(incidentID, severity, contextText)
	content := fmt.Sprintf(
		"事故排查结果（%s）\n- 严重级别：%s\n- 简短总结：%s\n- 建议动作：%s",
		incidentID,
		severity,
		summary,
		strings.Join(actions, "；"),
	)

	return map[string]interface{}{
		"skill_id":    in.SkillID,
		"version":     in.Version,
		"incident_id": incidentID,
		"severity":    severity,
		"summary":     summary,
		"actions":     actions,
		"content":     content,
		"trace_id":    in.TraceID,
		"executor":    "incident-triage",
	}, nil
}

func detectSeverity(text string) string {
	t := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(t, "宕机"), strings.Contains(t, "中断"), strings.Contains(t, "无法访问"), strings.Contains(t, "严重"), strings.Contains(t, "p0"), strings.Contains(t, "critical"), strings.Contains(t, "outage"):
		return "high"
	case strings.Contains(t, "降级"), strings.Contains(t, "告警"), strings.Contains(t, "超时"), strings.Contains(t, "error"), strings.Contains(t, "p1"), strings.Contains(t, "warning"):
		return "medium"
	default:
		return "low"
	}
}

func buildActions(severity string, text string) []string {
	actions := []string{
		"先确认影响范围（用户/区域/功能）并冻结变更",
		"检查最近发布与依赖变更，优先回滚可疑变更",
		"补充事故时间线与根因证据，形成复盘项",
	}
	if severity == "high" {
		actions = append([]string{"立即升级到值班负责人并启动应急预案"}, actions...)
	}
	if strings.Contains(strings.ToLower(text), "数据库") || strings.Contains(strings.ToLower(text), "db") {
		actions = append(actions, "重点核查数据库连接池、慢查询与锁等待")
	}
	return actions
}

func buildSummary(incidentID, severity, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "当前信息不足，建议先补充事故现象、影响范围和发生时间。"
	}
	if len(text) > 90 {
		text = text[:90] + "..."
	}
	return fmt.Sprintf("已完成初步排查，判定级别为 %s，建议按优先级执行止损与回滚检查。上下文：%s", severity, text)
}
