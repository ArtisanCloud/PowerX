package eino

import (
	"encoding/json"
	"fmt"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
	"time"
)

/* ================= async helpers ================= */

func (a *AgentClient) setStatus(id string, st *agentschema.ExecutionStatus) {
	a.asyncMu.Lock()
	defer a.asyncMu.Unlock()
	cp := *st
	a.statusByExec[id] = &cp
}

func (a *AgentClient) setResult(id string, r *agentschema.ExecutionResult) {
	a.asyncMu.Lock()
	defer a.asyncMu.Unlock()
	a.resultByExec[id] = r
}

func (a *AgentClient) getStart(id string) *time.Time {
	a.asyncMu.RLock()
	defer a.asyncMu.RUnlock()
	if st, ok := a.statusByExec[id]; ok {
		return st.StartedAt
	}
	return nil
}

/* ================= 别名 & 小工具 ================= */

func (a *AgentClient) aliasOf(in string) (string, bool) {
	a.flowMu.RLock()
	defer a.flowMu.RUnlock()
	alt, ok := a.flowAliases[strings.TrimSpace(in)]
	return alt, ok
}

func (a *AgentClient) SetFlowAliases(m map[string]string) {
	a.flowMu.Lock()
	defer a.flowMu.Unlock()
	for k, v := range m {
		ks := strings.TrimSpace(k)
		vs := strings.TrimSpace(v)
		if ks != "" && vs != "" {
			a.flowAliases[ks] = vs
		}
	}
}

/* ================= 小工具（支持 IO.InMap/OutMap 点路径） ================= */

func mergeCtx(base flowschema.Context, overlay map[string]any) flowschema.Context {
	out := flowschema.Context{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

func getByPathCtx(ctx flowschema.Context, path string) (any, bool) {
	// 允许从 ctx 根取，也允许从 _vars（步骤结果）取
	if v, ok := getByPathMap(map[string]any(ctx), path); ok {
		return v, true
	}
	// 兼容简写：当 path 以 "step_" 开头时，自动从 _vars 下取
	if strings.HasPrefix(path, "step_") {
		if vars, _ := ctx["_vars"].(map[string]any); vars != nil {
			return getByPathMap(vars, path)
		}
	}
	return nil, false
}

func getByPathMap(m map[string]any, path string) (any, bool) {
	cur := any(m)
	for _, p := range strings.Split(path, ".") {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		nxt, ok := obj[p]
		if !ok {
			return nil, false
		}
		cur = nxt
	}
	return cur, true
}

func setByPathCtx(ctx flowschema.Context, path string, val any) {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return
	}
	cur := map[string]any(ctx)
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = val
			return
		}
		nxt, ok := cur[p]
		if !ok {
			nm := map[string]any{}
			cur[p] = nm
			cur = nm
			continue
		}
		asMap, ok := nxt.(map[string]any)
		if !ok {
			nm := map[string]any{}
			cur[p] = nm
			cur = nm
			continue
		}
		cur = asMap
	}
}

func errMsg(err error, res *agentschema.ExecutionResult) string {
	if err != nil {
		return err.Error()
	}
	if res != nil && !res.Success && res.Error != "" {
		return res.Error
	}
	return ""
}

// LoadFlowAliases 从 JSON 或 YAML 文件加载别名映射：{"from":"to", ...}
func (a *AgentClient) LoadFlowAliases(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read alias file failed: %w", err)
	}
	var m map[string]string

	switch {
	case strings.HasSuffix(strings.ToLower(path), ".json"):
		if err := json.Unmarshal(b, &m); err != nil {
			return fmt.Errorf("parse json failed: %w", err)
		}
	case strings.HasSuffix(strings.ToLower(path), ".yaml"), strings.HasSuffix(strings.ToLower(path), ".yml"):
		type YAML map[string]string
		var y YAML
		if err := yaml.Unmarshal(b, &y); err != nil {
			return fmt.Errorf("parse yaml failed: %w", err)
		}
		m = map[string]string(y)
	default:
		return fmt.Errorf("unsupported alias file ext (use .json/.yaml)")
	}
	a.SetFlowAliases(m)
	return nil
}

/* ================= 本地分块（同步结果回放） ================= */

func chunkRunes(s string, n int) []string {
	if n <= 0 {
		n = 8
	}
	rs := []rune(s)
	var out []string
	for i := 0; i < len(rs); i += n {
		j := i + n
		if j > len(rs) {
			j = len(rs)
		}
		out = append(out, string(rs[i:j]))
	}
	return out
}
