package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"gorm.io/datatypes"
	// 你工程里的包，根据实际路径调整
	"github.com/ArtisanCloud/PowerX/config"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database" // 你的 DB 连接封装
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
)

var idParamRe = regexp.MustCompile(`\{[^}]*id[^}]*\}`) // 匹配 {id} / {userId} 等
var versionRe = regexp.MustCompile(`^v[0-9]+$`)

func main() {
	var openapiPath, source, introduced string
	var apply bool
	flag.StringVar(&openapiPath, "openapi", "./etc/openapi.json", "path to openapi json/yaml")
	flag.StringVar(&source, "source", "core", "permission source (core or plugin id)")
	flag.StringVar(&introduced, "introduced", "", "introduced version, e.g. v1.0.0")
	flag.BoolVar(&apply, "apply", false, "apply sync to DB (false=print payload)")
	flag.Parse()

	doc, err := loadOpenAPI(openapiPath)
	if err != nil {
		panic(err)
	}
	items := generatePermissionsFromOpenAPI(doc, source, introduced)

	if !apply {
		// 打印为 /sync 的请求载荷，供手工或 CI 调用
		payload := map[string]any{
			"source":     source,
			"introduced": introduced,
			"dry_run":    true,
			"items":      items,
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Printf("%s\n", b)
		return
	}

	// 直接落库（调用你的 Service）
	cfg := config.GetGlobalConfig()
	db, err := database.Connect(cfg.Database)
	if err != nil {
		panic(err)
	}
	svc := iamsvc.NewPermissionService(db)
	res, err := svc.SyncPermissions(context.Background(), source, introduced, items, false)
	if err != nil {
		panic(err)
	}
	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
}

func loadOpenAPI(path string) (*openapi3.T, error) {
	ldr := &openapi3.Loader{IsExternalRefsAllowed: true}
	ext := strings.ToLower(filepath.Ext(path))
	var doc *openapi3.T
	var err error
	if ext == ".yaml" || ext == ".yml" {
		doc, err = ldr.LoadFromFile(path)
	} else {
		doc, err = ldr.LoadFromFile(path) // kin-openapi 支持 json/yaml
	}
	if err != nil {
		return nil, err
	}
	if err = doc.Validate(ldr.Context); err != nil {
		// 放宽校验：非致命错误只告警
		fmt.Fprintf(os.Stderr, "[warn] openapi validation: %v\n", err)
	}
	return doc, nil
}

func generatePermissionsFromOpenAPI(doc *openapi3.T, source, introduced string) []dbm.Permission {
	var items []dbm.Permission

	// 关键修改：直接 range doc.Paths（map[string]*PathItem）
	for path, pi := range doc.Paths {
		for method, op := range operationsOf(pi) {
			perm := dbm.Permission{
				Module:     "core", // 或按你的模块名设置
				Resource:   guessResource(path),
				Action:     guessAction(method, path, op),
				Effect:     "allow",
				Status:     dbm.PermissionStatusActive,
				Source:     source,
				Introduced: introduced,
			}

			// label / module / type / endpoint / method -> meta
			label := op.Summary
			if label == "" {
				if op.OperationID != "" {
					label = op.OperationID
				} else {
					label = strings.ToUpper(method) + " " + path
				}
			}
			module := "API"
			if len(op.Tags) > 0 {
				module = op.Tags[0]
			}
			meta := map[string]any{
				"label":        label,
				"module":       module,
				"type":         "api",
				"api_endpoint": path,
				"http_method":  strings.ToUpper(method),
			}
			b, _ := json.Marshal(meta)
			perm.Meta = datatypes.JSON(b)

			items = append(items, perm)
		}
	}
	return items
}

func operationsOf(pi *openapi3.PathItem) map[string]*openapi3.Operation {
	m := map[string]*openapi3.Operation{}
	if pi.Get != nil {
		m["get"] = pi.Get
	}
	if pi.Post != nil {
		m["post"] = pi.Post
	}
	if pi.Delete != nil {
		m["delete"] = pi.Delete
	}
	if pi.Put != nil {
		m["put"] = pi.Put
	}
	if pi.Patch != nil {
		m["patch"] = pi.Patch
	}
	if pi.Options != nil {
		m["options"] = pi.Options
	}
	if pi.Head != nil {
		m["head"] = pi.Head
	}
	return m
}

func guessAction(method, path string, op *openapi3.Operation) string {
	m := strings.ToUpper(method)
	// 是否包含 ID 段
	hasID := idParamRe.MatchString(path)
	switch m {
	case "GET":
		if hasID {
			return "read"
		}
		return "list"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		// 从 operationId 提取一个动词前缀（如 exportUsers → export）
		if op != nil && op.OperationID != "" {
			id := strings.ToLower(op.OperationID)
			for _, v := range []string{"export", "sync", "approve", "reject"} {
				if strings.HasPrefix(id, v) {
					return v
				}
			}
		}
		return strings.ToLower(method)
	}
}

func singular(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	ls := strings.ToLower(s)
	if strings.HasSuffix(ls, "ies") {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(ls, "ses") { // e.g. processes
		return s[:len(s)-2]
	}
	if strings.HasSuffix(ls, "s") && !strings.HasSuffix(ls, "ss") {
		return s[:len(s)-1]
	}
	return s
}

// 期望：/api/v1/admin/iam/permissions -> iam.permission
//
//	/api/admin/tenants/:id -> tenant
func guessResource(path string) string {
	seg := strings.Split(strings.Trim(path, "/"), "/")
	// 去除通用前缀
	i := 0
	if i < len(seg) && seg[i] == "api" {
		i++
	}
	if i < len(seg) && versionRe.MatchString(seg[i]) {
		i++
	}
	if i < len(seg) && (seg[i] == "admin" || seg[i] == "open" || seg[i] == "public") {
		i++
	}

	// 安全兜底
	if i >= len(seg) {
		return "root"
	}

	// 去掉 path param 段
	for i < len(seg) && (strings.HasPrefix(seg[i], ":") || strings.HasPrefix(seg[i], "{")) {
		i++
	}
	if i >= len(seg) {
		return "root"
	}

	// 尝试双段组合（更贴近你的建模：iam.permission / role.member 等）
	s1 := singular(seg[i])
	s2 := ""
	if i+1 < len(seg) && !strings.HasPrefix(seg[i+1], ":") && !strings.HasPrefix(seg[i+1], "{") {
		s2 = singular(seg[i+1])
	}

	// 规则：如果首段是域（如 iam / role / dept 等）且后面是实体集合，就组合成 a.b
	if s1 == "iam" && s2 != "" {
		return strings.ToLower(s1 + "." + s2)
	}
	if (s2 == "permission" || s2 == "role" || s2 == "member" || s2 == "department" || s2 == "tenant") && s1 != "" {
		return strings.ToLower(s1 + "." + s2)
	}

	return strings.ToLower(s1)
}
