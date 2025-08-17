package handler

import (
	"context"
	"time"
)

// services/agent/handler/core_handler.go

// CoreDebugPass 统一的占位实现：读取 upstream_id，返回标准 map
func CoreDebugPass(ctx context.Context, ns *Namespace) (map[string]any, error) {
	idRef, _ := ns.Inputs["upstream_id"].(string)
	if idRef == "" {
		// 起点 seed 的场景容错，允许为空
		idRef = ns.Vars["upstream_id"].(string)
		if idRef == "" {
			idRef = "N/A"
		}
	}
	return map[string]any{
		"id":      idRef, // 保留 id 字段（便于 OutMap 或直接引用）
		"id_echo": idRef,
		"ok":      true,
		"ts_unix": time.Now().Unix(),
	}, nil
}

// CoreResponseFormat 简版（回显模板），你后面接入真正模板引擎再替换
func CoreResponseFormat(ctx context.Context, ns *Namespace) (map[string]any, error) {
	tpl, _ := ns.Inputs["template"].(string)
	return map[string]any{"render": tpl}, nil
}
