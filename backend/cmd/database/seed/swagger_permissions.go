// pkg/cmd/database/seed/swagger_permissions.go
package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	apikeypermissions "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	"gorm.io/gorm"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

var allowHTTP = map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true}

func SeedSwaggerPermissions(db *gorm.DB, swaggerPath string) error {
	b, err := os.ReadFile(swaggerPath)
	if err != nil {
		// 软跳过：文件不存在不算错误
		logger.InfoF(context.Background(), "[seed] swagger not found, skip: %s", swaggerPath)
		return nil
	}
	var doc struct {
		Paths map[string]map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("parse swagger: %w", err)
	}

	raw := 0
	var rows []dbm.Permission
	for path, methods := range doc.Paths {
		p := canonPath(path) // ★ 规范化路径，去掉多余斜杠、尾部斜杠
		for method := range methods {
			m := strings.ToLower(method)
			if !allowHTTP[m] {
				continue
			}
			raw++
			rows = append(rows, permFromPathAndMethod(p, strings.ToUpper(m)))
		}
	}
	if len(rows) == 0 {
		return nil
	}

	// ★ 关键：按 (module, resource, action) 去重，避免一次 upsert 命中同一行两次 → 21000
	rows = dedupPerms(rows)

	pr := repo.NewPermissionRepository(db)
	if err := pr.UpsertBatch(seedCtx(), rows); err != nil {
		return fmt.Errorf("upsert swagger perms: %w", err)
	}
	logger.InfoF(context.Background(), "[seed] swagger permissions upserted: %d (from %d)", len(rows), raw)
	return nil
}

func canonPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	// 合并重复斜杠
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	// 去尾部斜杠（根路径除外）
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// 关键修改点：plugin 和 module 的判别
func permFromPathAndMethod(path, method string) dbm.Permission {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	i := 0
	if i < len(segs) && (segs[i] == "api" || strings.HasPrefix(segs[i], "v")) {
		i++
	}
	isAdmin := false
	if i < len(segs) && segs[i] == "admin" {
		isAdmin = true
		i++
	}

	moduleName := "core"
	if i < len(segs) && segs[i] != "" && !strings.HasPrefix(segs[i], "{") {
		moduleName = segs[i] // iam / system / marketplace / tenant / ...
	}

	// module 规则：
	// - 后台管理且 plugin=system => system（平台级）
	// - 否则用 plugin 归类（iam/tenant/marketplace/...）
	module := moduleName
	if isAdmin && moduleName == "system" {
		module = "system"
	}

	// resource 按两段
	res := "misc"
	if i+1 < len(segs) {
		res = trimParam(segs[i]) + "." + trimParam(segs[i+1])
	} else if i < len(segs) {
		res = trimParam(segs[i])
	}

	act := "list"
	switch method {
	case "GET":
		if strings.Contains(path, "/:") || strings.Contains(path, "}") {
			act = "read"
		} else {
			act = "list"
		}
	case "POST":
		act = "create"
	case "PUT", "PATCH":
		act = "update"
	case "DELETE":
		act = "delete"
	}

	meta := map[string]any{
		"type":         "api",
		"module":       module,
		"label":        method + " " + path,
		"http_method":  method,
		"api_endpoint": path,
	}
	permission := dbm.Permission{
		Module:     moduleName, // 关键：来自路径
		Resource:   res,
		Action:     act,
		Effect:     "allow",
		Status:     dbm.PermissionStatusActive,
		Source:     moduleName, // 也用 module
		Introduced: config.GetSystemVersion(),
	}
	baseMetaBytes, _ := json.Marshal(meta)
	permission.Meta = baseMetaBytes
	permission.AllowAPIKey = apikeypermissions.DefaultAllowAPIKey(permission)
	if permission.AllowAPIKey {
		if apiMeta := apikeypermissions.BuildAPIKeyMeta(permission); len(apiMeta) > 0 {
			meta["api_key"] = apiMeta
		}
	}
	mb, _ := json.Marshal(meta)
	permission.Meta = mb
	return permission
}

func trimParam(s string) string {
	s = strings.TrimPrefix(s, ":")
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	return s
}

// —— 去重：按唯一键 (module, resource, action)
func dedupPerms(in []dbm.Permission) []dbm.Permission {
	type key struct {
		Module, Resource, Action string
	}
	m := make(map[key]dbm.Permission, len(in))
	for _, p := range in {
		k := key{
			Module:   strings.TrimSpace(p.Module),
			Resource: strings.TrimSpace(p.Resource),
			Action:   strings.TrimSpace(p.Action),
		}
		if old, ok := m[k]; ok {
			// 可选：合并字段，优先保留非空
			if len(p.Description) > 0 {
				old.Description = p.Description
			}
			if len(p.Meta) > 0 {
				old.Meta = p.Meta
			}
			if p.Status != "" {
				old.Status = p.Status
			}
			if p.Effect != "" {
				old.Effect = p.Effect
			}
			if p.Source != "" {
				old.Source = p.Source
			}
			if p.Introduced != "" {
				old.Introduced = p.Introduced
			}
			m[k] = old
		} else {
			m[k] = p
		}
	}
	out := make([]dbm.Permission, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
