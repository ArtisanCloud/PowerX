package handler

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

/* ====== 类型与常量 ====== */

type Scope int

const (
	ScopeCore     Scope = 1 // 核心（保护，不允许被覆盖）
	ScopeBusiness Scope = 2 // 业务（同名可覆盖 ThirdParty）
	ScopeThird    Scope = 3 // 第三方/扩展（优先级最低）
)

type HandlerFunc func(ctx context.Context, ns *Namespace) (map[string]any, error)

// Namespace: 执行时可读上下文
type Namespace struct {
	Inputs  map[string]any // 由 IO/ParamRefs 聚合出的“端口输入”，用于 handler 的主要入参
	Vars    map[string]any // 上下文变量（调用端拼的），如当前用户、租户、feature flags 等
	History any            // buildHistoryView 输出
}

// 元信息
type HandlerMeta struct {
	Scope       Scope
	Owner       string
	Description string
}

// 描述符（用于列表/调试）
type Descriptor struct {
	Use  string
	Meta HandlerMeta
}

/* ====== 注册表实现 ====== */

type HandlerRegistry struct {
	mu       sync.RWMutex
	items    map[string]registered // 归一化后 key -> 注册项
	useIndex map[string]string     // 原始 use -> 归一化 key（便于查询）
}

type registered struct {
	fn   HandlerFunc
	meta HandlerMeta
}

func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		items:    make(map[string]registered),
		useIndex: make(map[string]string),
	}
}

// 归一化 use：短名补全到 core 命名体系；统一小写
func normalizeUse(use string) string {
	u := strings.TrimSpace(strings.ToLower(use))
	if u == "" {
		return ""
	}
	// 兼容短名前缀：debug.seed -> core.debug.seed
	if !strings.Contains(u, ".") {
		u = "core." + u
	} else if !strings.HasPrefix(u, "core.") && !strings.HasPrefix(u, "biz.") && !strings.HasPrefix(u, "ext.") {
		// 没带明确前缀的，一律归到 core.*
		u = "core." + u
	}
	return u
}

// Register：带 Scope 的注册（含保护策略）
func (r *HandlerRegistry) Register(use string, fn HandlerFunc, meta HandlerMeta) error {
	if fn == nil {
		return fmt.Errorf("handler func is nil")
	}
	key := normalizeUse(use)
	if key == "" {
		return fmt.Errorf("invalid use")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 冲突保护：禁止覆盖 core
	if old, ok := r.items[key]; ok {
		if old.meta.Scope == ScopeCore && meta.Scope != ScopeCore {
			return fmt.Errorf("handler '%s' is protected (core)", key)
		}
		// 其他覆盖规则：Business 可覆盖 Third；Core 只能被 Core 覆盖（同源升级）
		if old.meta.Scope < meta.Scope {
			// 旧的优先级更高，拒绝
			return fmt.Errorf("handler '%s' exists with higher scope", key)
		}
	}

	r.items[key] = registered{fn: fn, meta: meta}
	r.useIndex[use] = key
	return nil
}

// RegisterLegacy：兼容老签名
func (r *HandlerRegistry) RegisterLegacy(
	use string,
	fn func(ctx context.Context, params map[string]any) (map[string]any, error),
	owner, desc string,
) error {
	wrapped := func(ctx context.Context, ns *Namespace) (map[string]any, error) {
		// 旧版只认识 params；我们把 Inputs 合并 Vars 给它
		merged := make(map[string]any, len(ns.Inputs)+len(ns.Vars))
		for k, v := range ns.Inputs {
			merged[k] = v
		}
		for k, v := range ns.Vars {
			merged[k] = v
		}
		return fn(ctx, merged)
	}
	return r.Register(use, wrapped, HandlerMeta{
		Scope:       ScopeThird,
		Owner:       owner,
		Description: desc,
	})
}

// Execute：查找并执行
func (r *HandlerRegistry) Execute(ctx context.Context, use string, ns *Namespace, preferred Scope) (map[string]any, error) {
	key := normalizeUse(use)

	r.mu.RLock()
	item, ok := r.items[key]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("handler not found: %s", key)
	}
	// 可扩展：preferred 影响多版本分发（当前只有单注册，略过）
	return item.fn(ctx, ns)
}

/* ====== 查询/统计 ====== */

func (r *HandlerRegistry) GetMeta(use string) (HandlerMeta, bool) {
	key := normalizeUse(use)
	r.mu.RLock()
	defer r.mu.RUnlock()
	it, ok := r.items[key]
	if !ok {
		return HandlerMeta{}, false
	}
	return it.meta, true
}

func (r *HandlerRegistry) Has(use string, _ Scope) bool {
	key := normalizeUse(use)
	r.mu.RLock()
	_, ok := r.items[key]
	r.mu.RUnlock()
	return ok
}

func (r *HandlerRegistry) Count(scope Scope) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if scope == 0 {
		return len(r.items)
	}
	n := 0
	for _, it := range r.items {
		if it.meta.Scope == scope {
			n++
		}
	}
	return n
}

func (r *HandlerRegistry) Names(scope Scope) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.items))
	for use, it := range r.items {
		if scope == 0 || it.meta.Scope == scope {
			out = append(out, use)
		}
	}
	sort.Strings(out)
	return out
}

func (r *HandlerRegistry) List(scope Scope) []Descriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Descriptor, 0, len(r.items))
	for use, it := range r.items {
		if scope == 0 || it.meta.Scope == scope {
			out = append(out, Descriptor{Use: use, Meta: it.meta})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Use < out[j].Use })
	return out
}
