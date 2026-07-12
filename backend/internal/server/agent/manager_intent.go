package agent

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// services/agent/manager_intent.go

// SetIntentStrategies 原子替换策略列表，并设置阈值。
// - strategies: 顺序即执行顺序（建议：Rule -> Embedding -> LLM）
// - low/high:   低/高阈值，若不合法会被修正；若 low > high 则交换
func (m *Manager) SetIntentStrategies(strategies []contract.IntentStrategy, low, high float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清洗阈值到 [0,1] 区间
	if low < 0 {
		low = 0
	}
	if low > 1 {
		low = 1
	}
	if high < 0 {
		high = 0
	}
	if high > 1 {
		high = 1
	}
	if low > high {
		low, high = high, low
	}

	// 过滤掉 nil，并做浅拷贝（避免外部切片被后续修改）
	clean := make([]contract.IntentStrategy, 0, len(strategies))
	for _, st := range strategies {
		if st != nil {
			clean = append(clean, st)
		}
	}

	// 可选：去重（按 Name 去重，保持第一次出现的顺序）
	// 如果不需要去重，直接删除以下这一段。
	type named interface{ Name() string }
	seen := map[string]struct{}{}
	dedup := make([]contract.IntentStrategy, 0, len(clean))
	for _, st := range clean {
		if n, ok := st.(named); ok {
			if _, hit := seen[n.Name()]; hit {
				continue
			}
			seen[n.Name()] = struct{}{}
		}
		dedup = append(dedup, st)
	}

	// 原子替换
	m.intentStrategies = slices.Clone(dedup)
	m.intentLow = low
	m.intentHigh = high
}

// 可选只读 Getter（便于 handler/日志调试）
func (m *Manager) IntentLow() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.intentLow
}
func (m *Manager) IntentHigh() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.intentHigh
}
func (m *Manager) IntentStrategyNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.intentStrategies))
	for _, st := range m.intentStrategies {
		if st == nil {
			continue
		}
		// 尝试拿 Name（不是必须）
		type named interface{ Name() string }
		if n, ok := st.(named); ok {
			out = append(out, n.Name())
		} else {
			out = append(out, "unknown")
		}
	}
	return out
}

// SetPlannerRules 暂存布线规则，供 Planner/测试使用。
func (m *Manager) SetPlannerRules(rules []schemas.WireRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(rules) == 0 {
		m.plannerRules = nil
		return
	}
	m.plannerRules = make([]schemas.WireRule, len(rules))
	copy(m.plannerRules, rules)
}

// ===== 工具：分句 =====
var splitRe = regexp.MustCompile(
	`(?i)[，、。；;,.!?！？/]|(?:\s*(?:然后|并且|再|接着|同时|顺便|以及|最后|和|跟|与|或|或者|并|and|&)\s*)`,
)

// ----- 工具：内联正则识别（可选保留）-----
func parseInlineRegex(s string) (pattern string, ok bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "re:") {
		return strings.TrimSpace(s[3:]), true
	}
	if len(s) >= 2 && s[0] == '/' && s[len(s)-1] == '/' {
		return s[1 : len(s)-1], true
	}
	return "", false
}

func normType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "keyword", "key", "kw":
		return "keyword"
	case "pattern", "regex", "re":
		return "pattern"
	default:
		return t
	}
}

func (m *Manager) RuleMatch(ctx context.Context, text string) (*schemas.IntentResult, error) {
	// 拿快照，避免长时间持锁
	m.mu.RLock()
	routes := make(map[string]routeRecord, len(m.routesByFlow))
	for k, v := range m.routesByFlow {
		routes[k] = v
	}
	low := m.intentLow
	m.mu.RUnlock()

	matchReason := "no match"
	best := &schemas.IntentResult{
		Matched:  false,
		Score:    0,
		Strategy: "rule",
		Reason:   matchReason,
	}

	raw := text
	rawL := strings.ToLower(raw)

	for flowID, rec := range routes {
		spec := rec.Spec
		if spec == nil || len(spec.Matchers) == 0 {
			continue
		}

		matched := false
		score := 0.0

		for _, mt := range spec.Matchers {
			//fmt2.Dump("match strategy", mt)
			switch normType(string(mt.Type)) {
			case "keyword":
				// 规范化工具：仅去除所有空白并小写（ASCII）；中文不受影响
				canon := func(s string) string {
					s = strings.ToLower(s)
					// 去所有空白（空格/制表/换行）
					b := make([]rune, 0, len(s))
					for _, r := range s {
						if !unicode.IsSpace(r) {
							b = append(b, r)
						}
					}
					return string(b)
				}

				rawCanon := canon(raw) // 原文本规范化一份

				for _, tok := range mt.Any {
					tok = strings.TrimSpace(tok)
					if tok == "" {
						continue
					}

					if pat, ok := parseInlineRegex(tok); ok {
						// 内联正则：维持你原来的逻辑
						if ok2, err := regexp.MatchString(pat, raw); err == nil && ok2 {
							matched = true
							base := 0.72 + 0.01*float64(mt.Priority)
							if base > score {
								score = base
								matchReason = "keyword:re"
							}
						}
						continue
					}

					// ① 原逻辑：大小写无关包含
					if strings.Contains(rawL, strings.ToLower(tok)) {
						matched = true
						base := 0.65 + 0.01*float64(mt.Priority)
						if base > score {
							score = base
							matchReason = "keyword"
						}
						continue
					}

					// ② 新增副通道：去空格的小写包含（解决 "task 4" vs "task4"）
					if strings.Contains(rawCanon, canon(tok)) {
						matched = true
						base := 0.66 + 0.01*float64(mt.Priority) // 略高一丁点，区分来源
						if base > score {
							score = base
							matchReason = "keyword:canon"
						}
						continue
					}
				}
			case "pattern":
				// 标准正则：优先用 mt.Regex；若你的结构体还有 mt.Pattern，可一起兼容
				pat := strings.TrimSpace(mt.Regex)
				if pat == "" {
					// 兼容：如果结构体有 Pattern 字段，可解注释
					// pat = strings.TrimSpace(mt.Pattern)
				}
				if pat == "" {
					continue
				}

				// 用原文本匹配，是否忽略大小写由正则写法（可加 (?i)）决定
				if ok2, err := regexp.MatchString(pat, raw); err == nil && ok2 {
					matched = true
					base := 0.72 + 0.01*float64(mt.Priority)
					if base > score {
						score = base
						matchReason = "pattern matched"
					}
				}
			}
		}

		if matched && score > best.Score {
			*best = schemas.IntentResult{
				Matched:  true,
				FlowID:   flowID,
				AgentID:  rec.AgentID,
				Score:    score,
				Strategy: "rule",
				Reason:   matchReason,
			}
		}
	}

	// 阈值判定（与原有逻辑一致）
	if best.Score < low {
		best.Matched = false
		best.Reason = "score below low threshold"
	}
	return best, nil
}

func (m *Manager) splitUtterance(text string) []string {
	parts := splitRe.Split(text, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		out = []string{strings.TrimSpace(text)}
	}
	if intentDebug {
		dlog("splitUtterance: %q -> %v", text, out)
	}
	return out
}

func (m *Manager) DetectIntent(ctx context.Context, text string) (*schemas.IntentResult, error) {
	tasks, err := m.DetectTasks(ctx, text)
	if err != nil {
		// 统一返回一个未命中，但带 reason 的结果，避免上层判空
		return &schemas.IntentResult{
			Matched:  false,
			Strategy: "aggregate",
			Reason:   err.Error(),
			Score:    0,
		}, nil
	}
	if len(tasks) == 0 {
		return &schemas.IntentResult{
			Matched:  false,
			Strategy: "aggregate",
			Reason:   "no task detected",
			Score:    0,
		}, nil
	}

	best := tasks[0] // DetectTasks 已按分数降序
	// 用阈值决定是否“命中”。一般：>= high 视为命中；介于 [low, high) 可认为候选
	matched := best.Score >= m.intentHigh

	return &schemas.IntentResult{
		Matched:  matched,
		FlowID:   best.FlowID,
		AgentID:  best.AgentID,
		Score:    best.Score,
		Strategy: best.Strategy, // 也可以写 "aggregate:"+best.Strategy，看你偏好
		Reason:   best.Reason,
	}, nil
}

// ===== 多意图识别（含去重/排序）=====
func (m *Manager) DetectTasks(ctx context.Context, text string) ([]schemas.DetectedTask, error) {
	clauses := m.splitUtterance(text)

	byFlow := map[string]schemas.DetectedTask{}
	idSeq := 0

	// （可选）打印配置
	if intentDebug {
		m.mu.RLock()
		names := make([]string, 0, len(m.intentStrategies))
		for _, st := range m.intentStrategies {
			if st == nil {
				continue
			}
			type named interface{ Name() string }
			if n, ok := st.(named); ok {
				names = append(names, n.Name())
			} else {
				names = append(names, "unknown")
			}
		}
		low, high := m.intentLow, m.intentHigh
		m.mu.RUnlock()
		dlog("strategies=%v  thresholds low=%.2f high=%.2f", names, low, high)
	}

	for _, c := range clauses {
		dlog("clause=%q", c)

		// —— 可选：打印“本子句所有命中的 rule（不影响结果，纯诊断）”
		if intentDebug && intentDebugAllRuleHits {
			hits := m.scanAllRuleMatches(ctx, c) // 见下文 helper
			if len(hits) == 0 {
				dlog("  rule-scan: no hits")
			} else {
				for _, h := range hits {
					dlog("  rule-scan HIT flow=%s score=%.2f reason=%s", h.FlowID, h.Score, h.Reason)
				}
			}
		}

		for _, st := range m.intentStrategies {
			r, _ := st.Match(ctx, c)
			if intentDebug {
				if r == nil {
					dlog("  -> %-10s nil-result", getStrategyName(st))
				} else {
					dlog("  -> %-10s matched=%v flow=%s score=%.2f reason=%s",
						getStrategyName(st), r.Matched, r.FlowID, r.Score, r.Reason)
				}
			}

			if r == nil || !r.Matched {
				continue
			}
			if r.Score < m.intentLow {
				dlog("     (drop: below low=%.2f)", m.intentLow)
				continue
			}
			if old, ok := byFlow[r.FlowID]; ok {
				if r.Score > old.Score {
					old.Score = r.Score
					old.Strategy = r.Strategy
					old.Reason = r.Reason
					old.AgentID = r.AgentID
					byFlow[r.FlowID] = old
					dlog("     update keep flow=%s newScore=%.2f", r.FlowID, r.Score)
				}
				continue
			}
			idSeq++
			byFlow[r.FlowID] = schemas.DetectedTask{
				TaskID:   fmt.Sprintf("t%d", idSeq),
				FlowID:   r.FlowID,
				AgentID:  r.AgentID,
				Score:    r.Score,
				Strategy: r.Strategy,
				Reason:   r.Reason,
			}
			dlog("     keep flow=%s score=%.2f", r.FlowID, r.Score)
		}
	}

	out := make([]schemas.DetectedTask, 0, len(byFlow))
	for _, v := range byFlow {
		out = append(out, v)
	}

	// 统一候选池回退：当策略链未产出任务时，从候选池做轻量召回（避免仅依赖 flow 路由）。
	if len(out) == 0 {
		out = m.detectTasksFromUnifiedCandidates(ctx, text)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })

	if intentDebug {
		if len(out) == 0 {
			dlog("FINAL: no task detected")
		} else {
			for _, t := range out {
				dlog("FINAL keep: flow=%s score=%.2f strategy=%s reason=%s taskID=%s",
					t.FlowID, t.Score, t.Strategy, t.Reason, t.TaskID)
			}
		}
	}
	return out, nil
}

func (m *Manager) detectTasksFromUnifiedCandidates(ctx context.Context, text string) []schemas.DetectedTask {
	cands := m.BuildToolCallCandidatesWithContext(CandidateBuildContextFromRequest(ctx), 64)
	if len(cands) == 0 {
		return nil
	}
	queryTokens := tokenizeCandidateText(text)
	back := make(map[string]ToolCallCandidate, len(cands))
	type scoredItem struct {
		key         string
		score       float64
		reason      string
		matchTokens []string
	}
	scored := make([]scoredItem, 0, len(cands))
	for _, c := range cands {
		ref := strings.TrimSpace(c.NodeRef)
		if ref == "" {
			ref = strings.TrimSpace(c.FlowID)
		}
		if ref == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(c.Name))
		if key == "" {
			key = strings.ToLower(ref)
		}
		back[key] = c
		doc := strings.ToLower(strings.TrimSpace(strings.Join([]string{
			key,
			c.Name,
			c.NodeKind,
			c.NodeRef,
			c.Description,
			c.SemanticText,
			strings.Join(c.IntentHints, " "),
			strings.Join(c.Tags, " "),
		}, " ")))
		matchTokens := make([]string, 0, len(queryTokens))
		for _, tok := range queryTokens {
			if strings.Contains(doc, tok) {
				matchTokens = append(matchTokens, tok)
			}
		}
		reason := "candidate_recall"
		if len(matchTokens) > 0 {
			reason = "token_match:" + strings.Join(matchTokens, ",")
		}
		score := float64(len(matchTokens))
		if len(queryTokens) > 0 {
			denominator := len(queryTokens)
			// 中文 2/3-gram token 数会明显放大，封顶分母避免召回分数被稀释到不可用。
			if denominator > 8 {
				denominator = 8
			}
			score = score / float64(denominator)
		}
		scored = append(scored, scoredItem{
			key:         key,
			score:       score,
			reason:      reason,
			matchTokens: matchTokens,
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].key < scored[j].key
		}
		return scored[i].score > scored[j].score
	})
	if len(scored) > 6 {
		scored = scored[:6]
	}
	if len(scored) == 0 {
		return nil
	}
	out := make([]schemas.DetectedTask, 0, len(scored))
	for i, s := range scored {
		if len(s.matchTokens) == 0 {
			continue
		}
		if s.score < m.intentLow {
			continue
		}
		c := back[s.key]
		ref := strings.TrimSpace(c.NodeRef)
		if ref == "" {
			ref = strings.TrimSpace(c.FlowID)
		}
		if ref == "" {
			continue
		}
		params := map[string]interface{}{
			"_node_kind":      strings.TrimSpace(c.NodeKind),
			"_node_ref":       ref,
			"_source_scope":   strings.TrimSpace(c.SourceScope),
			"_candidate_name": c.Name,
			"_candidate_desc": strings.TrimSpace(c.Description),
			"user_message":    strings.TrimSpace(text),
		}
		if action := inferCandidateAction(text, c.Actions); action != "" {
			params["action"] = action
			enrichCandidateRecallParams(params, text, action, c)
		}
		if missingRequiredCandidateParams(params, c.RequiredArgs) {
			continue
		}
		out = append(out, schemas.DetectedTask{
			TaskID:   fmt.Sprintf("u%d", i+1),
			FlowID:   ref,
			AgentID:  c.AgentID,
			Score:    s.score,
			Strategy: "candidate_recall:" + strings.TrimSpace(c.NodeKind),
			Reason:   s.reason,
			Params:   params,
		})
	}
	return out
}

func missingRequiredCandidateParams(params map[string]interface{}, required []string) bool {
	for _, raw := range required {
		key := strings.ToLower(strings.TrimSpace(raw))
		if key == "" {
			continue
		}
		value, ok := params[key]
		if !ok {
			return true
		}
		if strings.TrimSpace(fmt.Sprint(value)) == "" || strings.TrimSpace(fmt.Sprint(value)) == "<nil>" {
			return true
		}
	}
	return false
}

func enrichCandidateRecallParams(params map[string]interface{}, text, action string, c ToolCallCandidate) {
	if params == nil {
		return
	}
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return
	}
	if value, ok := params["template_ref"]; ok && strings.TrimSpace(fmt.Sprint(value)) != "" && strings.TrimSpace(fmt.Sprint(value)) != "<nil>" {
		return
	}
	if ref := extractTemplateRefFromUserText(text, action); ref != "" {
		params["template_ref"] = ref
	}
}

func extractTemplateRefFromUserText(text, action string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	triggers := actionTriggerWords(action)
	start := -1
	for _, trigger := range triggers {
		idx := strings.LastIndex(text, trigger)
		if idx >= 0 && idx+len(trigger) > start {
			start = idx + len(trigger)
		}
	}
	if start < 0 || start >= len(text) {
		return ""
	}
	ref := strings.TrimSpace(text[start:])
	ref = strings.TrimLeft(ref, "掉了掉一下把将：:，,。 .")
	ref = strings.TrimRight(ref, "。！？!?，,；; ")
	for _, suffix := range []string{"这个模板", "该模板", "这个模版", "该模版"} {
		ref = strings.TrimSuffix(ref, suffix)
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.Contains(ref, "模板") || strings.Contains(ref, "模版") {
		return ref
	}
	return ""
}

func actionTriggerWords(action string) []string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "delete":
		return []string{"删除掉", "删掉", "删除", "移除", "remove", "delete"}
	case "get":
		return []string{"查询", "查看", "获取", "看看", "看一下", "get", "read", "show"}
	case "update":
		return []string{"更新", "修改", "编辑", "改一下", "update", "edit"}
	default:
		return nil
	}
}

func inferCandidateAction(text string, actions []string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" || len(actions) == 0 {
		return ""
	}
	actionSet := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		action = strings.ToLower(strings.TrimSpace(action))
		if action != "" {
			actionSet[action] = struct{}{}
		}
	}
	for _, item := range []struct {
		action string
		words  []string
	}{
		{action: "create", words: []string{"创建", "新建", "新增", "生成", "create", "add"}},
		{action: "get", words: []string{"查询", "查看", "获取", "详情", "get", "read", "show"}},
		{action: "update", words: []string{"更新", "修改", "编辑", "改成", "update", "edit"}},
		{action: "delete", words: []string{"删除", "移除", "删掉", "delete", "remove"}},
		{action: "list", words: []string{"列表", "列出", "有哪些", "全部", "list"}},
	} {
		if _, ok := actionSet[item.action]; !ok {
			continue
		}
		for _, word := range item.words {
			if strings.Contains(text, strings.ToLower(word)) {
				return item.action
			}
		}
	}
	return ""
}

func tokenizeCandidateText(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}

	segments := strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	seen := make(map[string]struct{}, len(segments)*3)
	out := make([]string, 0, len(segments)*3)
	push := func(tok string) {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			return
		}
		if _, ok := seen[tok]; ok {
			return
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		push(seg)
		rs := []rune(seg)
		if len(rs) < 2 {
			continue
		}
		// 中文场景：补充 2-gram / 3-gram，解决无空格文本召回弱问题。
		if containsHanRune(rs) {
			for _, n := range []int{2, 3} {
				if len(rs) < n {
					continue
				}
				for i := 0; i <= len(rs)-n; i++ {
					push(string(rs[i : i+n]))
				}
			}
		}
	}
	return out
}

func containsHanRune(rs []rune) bool {
	for _, r := range rs {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func getStrategyName(st contract.IntentStrategy) string {
	type named interface{ Name() string }
	if n, ok := st.(named); ok {
		return n.Name()
	}
	return "unknown"
}

// ===== 计划构建：依赖补全 + 分层 + 参数连线 =====

// 读路由表里的 IntentSpec
func (m *Manager) getSpec(flowID string) (*schemas.IntentSpec, string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.routesByFlow[flowID]
	if !ok {
		return nil, "", false
	}
	return rec.Spec, rec.AgentID, true
}

func (m *Manager) getIOSets(sp *schemas.IntentSpec) (map[string]struct{}, map[string]struct{}) {
	inSet, outSet := map[string]struct{}{}, map[string]struct{}{}
	if sp == nil || sp.Metadata == nil || sp.Metadata.IO == nil {
		return inSet, outSet
	}

	for _, p := range sp.Metadata.IO.Inputs {
		if p.Name != "" {
			inSet[p.Name] = struct{}{}
		}
	}
	for _, p := range sp.Metadata.IO.Outputs {
		if p.Name != "" {
			outSet[p.Name] = struct{}{}
		}
	}

	return inSet, outSet
}
