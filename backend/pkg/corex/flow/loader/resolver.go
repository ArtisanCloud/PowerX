package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// Resolver 负责：索引 -> 解析（单目录，无多目录合并）
type Resolver struct {
	Dir    string
	Loader *YAMLSpecLoader

	index        map[string]string                   // flowID -> 路径
	aliases      map[string]string                   // aliasID -> targetID（来自 ref）
	refOverrides map[string]*flowschema.FlowMetadata // aliasID -> overrides.metadata

	Debug bool // 环境变量 POWERX_FLOW_DEBUG=1 或 DEBUG=1 时开启
}

func NewResolver(dir string) *Resolver {
	dbg := os.Getenv("POWERX_FLOW_DEBUG") == "1" || os.Getenv("DEBUG") == "1"
	return &Resolver{
		Dir:          dir,
		Loader:       NewYAMLSpecLoader("."),
		index:        map[string]string{},
		aliases:      map[string]string{},
		refOverrides: map[string]*flowschema.FlowMetadata{},
		Debug:        dbg,
	}
}

func (r *Resolver) dbg(format string, args ...any) {
	if r.Debug {
		logger.DebugF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "flow.resolver"}), "[resolver] "+format, args...)
	}
}

// BuildIndex 扫描目录，建立索引 + 收集 ref 别名映射（✅ 只看顶层 Flow.Ref）
func (r *Resolver) BuildIndex() error {
	r.dbg("BuildIndex: scanning dir=%s", r.Dir)
	m, err := r.Loader.LoadBlueprintsFromDir(r.Dir)
	if err != nil {
		return err
	}
	for id, bp := range m {
		path := bp.FlowPath
		if path == "" {
			path = filepath.Join(r.Dir, id+".yaml")
		}
		r.index[id] = path

		if bp.Ref != nil && bp.Ref.Enable && strings.TrimSpace(bp.Ref.Target) != "" {
			r.aliases[id] = strings.TrimSpace(bp.Ref.Target)
			if bp.Ref.Overrides.Metadata != nil {
				r.refOverrides[id] = bp.Ref.Overrides.Metadata
			}
			r.dbg("index: alias %q -> %q (path=%s)", id, bp.Ref.Target, path)
		} else {
			r.dbg("index: flow  %q (path=%s)", id, path)
		}
	}

	if r.Debug {
		ids := make([]string, 0, len(r.index))
		for k := range r.index {
			ids = append(ids, k)
		}
		sort.Strings(ids)
		al := make([]string, 0, len(r.aliases))
		for k, v := range r.aliases {
			al = append(al, fmt.Sprintf("%s->%s", k, v))
		}
		sort.Strings(al)

		logCtx := logger.WithLogFields(context.Background(), map[string]interface{}{"module": "flow.resolver"})
		logger.DebugF(logCtx, "[resolver] index_count=%d alias_count=%d", len(ids), len(al))
		logger.DebugF(logCtx, "[resolver] index_ids(sample)=%s", sample(ids, 12))
		if len(al) > 0 {
			logger.DebugF(logCtx, "[resolver] aliases(sample)=%s", sample(al, 12))
		}
	}
	return nil
}

// ✅ 统一入口：全部走带环路检测/统一日志的 resolveInternal
func (r *Resolver) Resolve(flowID string) (*flowschema.Flow, error) {
	return r.resolveInternal(flowID, map[string]bool{})
}

// ✅ 只看顶层 Flow.Ref，递归展开 alias → target，合并 overrides，保留 alias 身份
func (r *Resolver) resolveInternal(flowID string, visiting map[string]bool) (*flowschema.Flow, error) {
	if visiting[flowID] {
		return nil, fmt.Errorf("circular ref detected at %s", flowID)
	}
	visiting[flowID] = true
	defer delete(visiting, flowID)

	r.dbg("Resolve: try=%s", flowID)

	// 先按索引加载蓝图（alias 文件本身也在索引中）
	if path, ok := r.index[flowID]; ok {
		f, err := r.Loader.LoadFlowBlueprint(path)
		if err != nil {
			r.dbg("Resolve: load failed id=%s path=%s err=%v", flowID, path, err)
			return nil, err
		}

		// 非 ref：直接返回
		if f.Ref == nil || !f.Ref.Enable || strings.TrimSpace(f.Ref.Target) == "" {
			r.dbg("Resolve: ok (plain) id=%s nodes=%d", flowID, len(f.Nodes))
			return f, nil
		}

		// ref：递归解析 target
		targetID := strings.TrimSpace(f.Ref.Target)
		r.dbg("Resolve: alias id=%s -> target=%s", flowID, targetID)
		base, err := r.resolveInternal(targetID, visiting)
		if err != nil {
			r.dbg("Resolve: target failed id=%s target=%s err=%v", flowID, targetID, err)
			return nil, fmt.Errorf("resolve ref target(%s) failed: %w", targetID, err)
		}

		// 基于 target 拷贝，并改回 alias 身份 + 合并 overrides
		cp := *base
		cp.FlowID = f.FlowID
		cp.FlowPath = f.FlowPath
		cp.Metadata = mergeFlowMetadata(base.Metadata, r.refOverrides[flowID])

		// alias 上的 Name/Version/Description 优先
		if f.Name != "" {
			cp.Name = f.Name
		}
		if f.Version != "" {
			cp.Version = f.Version
		}
		if f.Description != "" {
			cp.Description = f.Description
		}

		r.dbg("Resolve: ok (alias) id=%s -> target=%s nodes=%d", flowID, targetID, len(cp.Nodes))
		return &cp, nil
	}

	// 兜底：按文件名直接加载（主要用于诊断）
	probe := filepath.Join(r.Dir, flowID+".yaml")
	if f, err := r.Loader.LoadFlowBlueprint(probe); err == nil {
		r.dbg("Resolve: ok (fallback file) id=%s file=%s nodes=%d", flowID, probe, len(f.Nodes))
		return f, nil
	}

	if r.Debug {
		keys := make([]string, 0, len(r.index))
		for k := range r.index {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		logCtx := logger.WithLogFields(context.Background(), map[string]interface{}{"module": "flow.resolver"})
		logger.DebugF(logCtx, "[resolver] Resolve: NOT FOUND id=%s ; have_ids(sample)=%s ; dir=%s", flowID, sample(keys, 20), r.Dir)
		if tgt, ok := r.aliases[flowID]; ok {
			logger.DebugF(logCtx, "[resolver] Resolve: alias entry exists id=%s -> target=%s BUT no file indexed (check loader/index)", flowID, tgt)
		}
	}
	return nil, fmt.Errorf("flow not found: %s", flowID)
}

/* ===== helpers ===== */

func sample(a []string, n int) string {
	if len(a) == 0 {
		return "[]"
	}
	if n > len(a) {
		n = len(a)
	}
	return "[" + strings.Join(a[:n], ", ") + func() string {
		if n < len(a) {
			return ", ..."
		}
		return ""
	}() + "]"
}

// 叠加 overrides.metadata 到 base（nil 安全）
func mergeFlowMetadata(base *flowschema.FlowMetadata, ov *flowschema.FlowMetadata) *flowschema.FlowMetadata {
	if base == nil && ov == nil {
		return nil
	}
	if base == nil {
		cp := *ov
		return &cp
	}
	cp := *base
	if ov == nil {
		return &cp
	}

	if ov.Domain != "" {
		cp.Domain = ov.Domain
	}
	if ov.Version != "" {
		cp.Version = ov.Version
	}
	if ov.Description != "" {
		cp.Description = ov.Description
	}
	if ov.Priority != 0 {
		cp.Priority = ov.Priority
	}
	if ov.Owner != "" {
		cp.Owner = ov.Owner
	}
	if ov.IsPublic {
		cp.IsPublic = true
	}
	if ov.ExtraInfo != nil {
		if cp.ExtraInfo == nil {
			cp.ExtraInfo = map[string]string{}
		}
		for k, v := range ov.ExtraInfo {
			cp.ExtraInfo[k] = v
		}
	}
	if len(ov.Tags) > 0 {
		cp.Tags = unionStrings(cp.Tags, ov.Tags)
	}
	if len(ov.Requires) > 0 {
		cp.Requires = unionStrings(cp.Requires, ov.Requires)
	}
	if ov.IO != nil {
		cp.IO = ov.IO
	}
	if ov.Intent != nil {
		cp.Intent = ov.Intent
	}
	return &cp
}

func unionStrings(a, b []string) []string {
	m := map[string]struct{}{}
	for _, x := range a {
		m[x] = struct{}{}
	}
	for _, y := range b {
		m[y] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
