// internal/openapi/minimal.go
package openapi

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// 增加：白名单方法、过滤函数、opId 生成
var allowMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true, "delete": true,
}

// 过滤：不进权限的路径
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
func opID(method, path string) string {
	// 稳定且唯一：METHOD + 正规化 path
	s := strings.ToUpper(method) + " " + path
	s = strings.ReplaceAll(s, ":", "_") // :id -> _id
	s = strings.ReplaceAll(s, "*", "star")
	s = strings.ReplaceAll(s, "{", "")
	s = strings.ReplaceAll(s, "}", "")
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
		// 首段作为 tag（admin/open去掉后再说，下面我们在 perm_gen 再做资源名规整）
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
