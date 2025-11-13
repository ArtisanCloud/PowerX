// internal/openapi/minimal.go
package openapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

type Info struct {
	Title   string
	Version string
	BaseURL string // 可选：如 "/" 或 "http://localhost:8077"
}

// 允许进入文档的方法（排除 HEAD/OPTIONS）
var allowMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true, "delete": true,
}

// 过滤：不进权限/文档的路径
func shouldSkipPath(p string) bool {
	switch {
	case strings.HasPrefix(p, "/swagger/"):
		return true
	case p == "/openapi.min.json":
		return true
	case strings.HasPrefix(p, "/_p/"): // 插件动态代理/静态文件
		return true
	case p == "/favicon.ico":
		return true
	default:
		return false
	}
}

var opCleanRe = regexp.MustCompile(`[^A-Za-z0-9 _\-/]`)

// 生成稳定且可读的 operationId
func opID(method, path string) string {
	// 例： "GET /api/admin/iam/roles/:id" -> "GET /api/admin/iam/roles/_id"
	s := strings.ToUpper(method) + " " + path
	s = strings.ReplaceAll(s, ":", "_") // :id -> _id（Gin 风格）
	s = strings.ReplaceAll(s, "*", "star")
	s = strings.ReplaceAll(s, "{", "")
	s = strings.ReplaceAll(s, "}", "")
	// 清理非常规字符，避免极端情况下出现不可控的符号
	s = opCleanRe.ReplaceAllString(s, "_")
	return s
}

func BuildMinimalDoc(r *gin.Engine, info Info) map[string]any {
	routes := r.Routes()
	paths := map[string]map[string]map[string]any{} // path -> method -> operation

	type item struct{ Method, Path string }
	var all []item
	for _, rt := range routes {
		m := strings.ToLower(rt.Method)
		p := rt.Path
		if !allowMethods[m] || shouldSkipPath(p) {
			continue
		}
		all = append(all, item{Method: m, Path: p})
	}

	// 为了输出的确定性，先按 path+method 排序再组装
	sort.Slice(all, func(i, j int) bool {
		if all[i].Path == all[j].Path {
			return all[i].Method < all[j].Method
		}
		return all[i].Path < all[j].Path
	})

	for _, it := range all {
		if _, ok := paths[it.Path]; !ok {
			paths[it.Path] = map[string]map[string]any{}
		}

		// 简单用首段作为 tag（更细的资源归类可在权限生成处处理）
		tag := "API"
		seg := strings.Split(strings.Trim(it.Path, "/"), "/")
		if len(seg) > 0 && seg[0] != "" && !strings.HasPrefix(seg[0], "{") {
			tag = seg[0]
		}

		paths[it.Path][it.Method] = map[string]any{
			"tags":        []string{tag},
			"summary":     opID(it.Method, it.Path),
			"operationId": opID(it.Method, it.Path),
			"responses": map[string]any{
				"200": map[string]any{"description": "OK"},
			},
		}
	}

	doc := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   coalesce(info.Title, "API"),
			"version": coalesce(info.Version, "v0.0.0"),
		},
		"servers": []map[string]any{},
		"paths":   paths,
	}
	if strings.TrimSpace(info.BaseURL) != "" {
		doc["servers"] = append(doc["servers"].([]map[string]any), map[string]any{"url": info.BaseURL})
	}
	applyStaticStubs(doc)
	return doc
}

// SaveMinimalDoc: 将最小 OpenAPI 文档写入到指定目录（swagger.json / swagger.yaml）
func SaveMinimalDoc(r *gin.Engine, info Info, outDir string) error {
	doc := BuildMinimalDoc(r, info)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	// JSON
	jb, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "swagger.json"), jb, 0o644); err != nil {
		return err
	}

	// YAML
	yb, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "swagger.yaml"), yb, 0o644); err != nil {
		return err
	}
	return nil
}

// MountMinimalOpenAPI: 挂载 /openapi.min.json 供 UI/外部工具访问
func MountMinimalOpenAPI(r *gin.Engine, info Info) {
	r.GET("/openapi.min.json", func(c *gin.Context) {
		c.JSON(200, BuildMinimalDoc(r, info))
	})
}

func coalesce(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}

func applyStaticStubs(doc map[string]any) {
	if len(agentModelHubStub) == 0 {
		return
	}
	var stub map[string]any
	if err := yaml.Unmarshal(agentModelHubStub, &stub); err != nil {
		fmt.Printf("openapi: failed to parse agent model hub stub: %v\n", err)
		return
	}
	mergeTags(doc, stub)
	mergePaths(doc, stub)
	mergeComponents(doc, stub)
}

func mergeTags(doc, stub map[string]any) {
	stubTags, ok := stub["tags"].([]any)
	if !ok || len(stubTags) == 0 {
		return
	}
	existing, _ := doc["tags"].([]any)
	seen := map[string]bool{}
	for _, raw := range existing {
		if tag, ok := raw.(map[string]any); ok {
			if name, _ := tag["name"].(string); name != "" {
				seen[name] = true
			}
		}
	}
	for _, raw := range stubTags {
		tag, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := tag["name"].(string)
		if name == "" || seen[name] {
			continue
		}
		existing = append(existing, tag)
		seen[name] = true
	}
	if len(existing) > 0 {
		doc["tags"] = existing
	}
}

func mergePaths(doc, stub map[string]any) {
	dst := ensureMap(doc, "paths")
	src, ok := stub["paths"].(map[string]any)
	if !ok {
		return
	}
	for path, val := range src {
		dst[path] = val
	}
}

func mergeComponents(doc, stub map[string]any) {
	src, ok := stub["components"].(map[string]any)
	if !ok {
		return
	}
	dst := ensureMap(doc, "components")
	for key, val := range src {
		subSrc, okSrc := val.(map[string]any)
		if !okSrc {
			dst[key] = val
			continue
		}
		subDst, okDst := dst[key].(map[string]any)
		if !okDst {
			dst[key] = val
			continue
		}
		for innerKey, innerVal := range subSrc {
			subDst[innerKey] = innerVal
		}
	}
}

func ensureMap(doc map[string]any, key string) map[string]any {
	if doc == nil {
		return map[string]any{}
	}
	if m, ok := doc[key].(map[string]any); ok {
		return m
	}
	m := map[string]any{}
	doc[key] = m
	return m
}
