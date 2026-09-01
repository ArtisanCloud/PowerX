package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

type stringListFlag []string

func (s *stringListFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringListFlag) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			*s = append(*s, p)
		}
	}
	return nil
}

type capabilityFile struct {
	Version      int               `yaml:"version"`
	Capabilities []capabilityEntry `yaml:"capabilities"`
}

type capabilityEntry struct {
	CapabilityID   string           `yaml:"capability_id"`
	Module         string           `yaml:"module"`
	Title          string           `yaml:"title"`
	Description    string           `yaml:"description,omitempty"`
	PermissionCode string           `yaml:"permission_code,omitempty"`
	AgentUsable    *bool            `yaml:"agent_usable,omitempty"`
	RiskLevel      string           `yaml:"risk_level,omitempty"`
	Categories     []string         `yaml:"categories,omitempty"`
	Intents        []string         `yaml:"intents,omitempty"`
	ToolScopes     []string         `yaml:"tool_scopes,omitempty"`
	Policy         capabilityPolicy `yaml:"policy"`
	Docs           []string         `yaml:"docs,omitempty"`
	Protocols      []protocolEntry  `yaml:"protocols"`
}

type capabilityPolicy struct {
	Prefer   string   `yaml:"prefer,omitempty"`
	Fallback []string `yaml:"fallback,omitempty"`
}

type protocolEntry struct {
	Channel       string `yaml:"channel"`
	Endpoint      string `yaml:"endpoint,omitempty"`
	Method        string `yaml:"method,omitempty"`
	RPC           string `yaml:"rpc,omitempty"`
	SchemaRef     string `yaml:"schema_ref,omitempty"`
	AuthType      string `yaml:"auth_type,omitempty"`
	ActorContext  string `yaml:"actor_context,omitempty"`
	ResourceScope string `yaml:"resource_scope,omitempty"`
	STSDirect     bool   `yaml:"sts_direct,omitempty"`
	ToolScope     string `yaml:"tool_scope,omitempty"`
}

type config struct {
	OpenAPIFiles []string
	ProtoInputs  []string
	GinSources   []string
	Out          string
	Prefix       string
	AuthType     string
	APIPrefix    string
	DryRun       bool
}

func main() {
	var openapiInputs stringListFlag
	var protoInputs stringListFlag
	var ginSources stringListFlag
	cfg := config{}
	flag.Var(&openapiInputs, "openapi", "OpenAPI json/yaml file (repeatable or comma-separated)")
	flag.Var(&protoInputs, "proto", "Proto file or directory (repeatable or comma-separated)")
	flag.Var(&ginSources, "gin-src", "Go source directory/file for Gin routes (repeatable or comma-separated)")
	flag.StringVar(&cfg.Out, "out", "backend/config/platform_capabilities/generated.auto.yaml", "output capability yaml path")
	flag.StringVar(&cfg.Prefix, "prefix", "com.corex", "capability_id prefix")
	flag.StringVar(&cfg.AuthType, "auth", "tenant_jwt", "default auth_type")
	flag.StringVar(&cfg.APIPrefix, "api-prefix", "/api/v1", "api prefix for generated Gin routes")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "print YAML to stdout instead of writing file")
	flag.Parse()

	cfg.OpenAPIFiles = openapiInputs
	cfg.ProtoInputs = protoInputs
	cfg.GinSources = ginSources

	if len(cfg.OpenAPIFiles) == 0 && len(cfg.ProtoInputs) == 0 && len(cfg.GinSources) == 0 {
		fatalf("at least one --openapi, --proto or --gin-src is required")
	}

	entries, err := buildCapabilities(cfg)
	if err != nil {
		fatalf("build capabilities failed: %v", err)
	}
	if len(entries) == 0 {
		fatalf("no capabilities generated")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].CapabilityID < entries[j].CapabilityID })

	out := capabilityFile{Version: 1, Capabilities: entries}
	raw, err := yaml.Marshal(out)
	if err != nil {
		fatalf("marshal yaml failed: %v", err)
	}

	if cfg.DryRun {
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "%s", raw)
		return
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Out), 0o755); err != nil {
		fatalf("create output dir failed: %v", err)
	}
	if err := os.WriteFile(cfg.Out, raw, 0o644); err != nil {
		fatalf("write output failed: %v", err)
	}
	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "generated %d capabilities -> %s", len(entries), cfg.Out)
}

func buildCapabilities(cfg config) ([]capabilityEntry, error) {
	entries := make([]capabilityEntry, 0, 256)
	seen := map[string]struct{}{}
	seenRoute := map[string]struct{}{}

	for _, path := range cfg.OpenAPIFiles {
		opEntries, err := genFromOpenAPI(path, cfg.Prefix, cfg.AuthType)
		if err != nil {
			return nil, err
		}
		for _, e := range opEntries {
			if _, ok := seen[e.CapabilityID]; ok {
				continue
			}
			seen[e.CapabilityID] = struct{}{}
			entries = append(entries, e)
			if len(e.Protocols) > 0 {
				rk := strings.ToUpper(strings.TrimSpace(e.Protocols[0].Method)) + " " + strings.TrimSpace(e.Protocols[0].Endpoint)
				seenRoute[rk] = struct{}{}
			}
		}
	}

	protoFiles, err := collectProtoFiles(cfg.ProtoInputs)
	if err != nil {
		return nil, err
	}
	for _, path := range protoFiles {
		psEntries, err := genFromProto(path, cfg.Prefix, cfg.AuthType)
		if err != nil {
			return nil, err
		}
		for _, e := range psEntries {
			if _, ok := seen[e.CapabilityID]; ok {
				continue
			}
			seen[e.CapabilityID] = struct{}{}
			entries = append(entries, e)
		}
	}

	ginFiles, err := collectGoFiles(cfg.GinSources)
	if err != nil {
		return nil, err
	}
	for _, path := range ginFiles {
		ginEntries, gerr := genFromGinSource(path, cfg.Prefix, cfg.AuthType, cfg.APIPrefix, seenRoute)
		if gerr != nil {
			return nil, gerr
		}
		for _, e := range ginEntries {
			if _, ok := seen[e.CapabilityID]; ok {
				continue
			}
			seen[e.CapabilityID] = struct{}{}
			entries = append(entries, e)
			if len(e.Protocols) > 0 {
				rk := strings.ToUpper(strings.TrimSpace(e.Protocols[0].Method)) + " " + strings.TrimSpace(e.Protocols[0].Endpoint)
				seenRoute[rk] = struct{}{}
			}
		}
	}

	return entries, nil
}

func genFromOpenAPI(path, prefix, auth string) ([]capabilityEntry, error) {
	ldr := &openapi3.Loader{IsExternalRefsAllowed: true}
	doc, err := ldr.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load openapi %s: %w", path, err)
	}

	entries := make([]capabilityEntry, 0, len(doc.Paths)*2)
	for p, item := range doc.Paths {
		ops := map[string]*openapi3.Operation{
			"GET":     item.Get,
			"POST":    item.Post,
			"PUT":     item.Put,
			"PATCH":   item.Patch,
			"DELETE":  item.Delete,
			"OPTIONS": item.Options,
			"HEAD":    item.Head,
		}
		for method, op := range ops {
			if op == nil {
				continue
			}
			module := moduleFromPath(p)
			action := "invoke"
			if op.OperationID != "" {
				action = slug(op.OperationID)
			} else {
				action = slug(method + "_" + p)
			}
			id := joinID(prefix, "rest", module, action)
			title := strings.TrimSpace(op.Summary)
			if title == "" {
				title = method + " " + p
			}
			desc := strings.TrimSpace(op.Description)
			if desc == "" {
				desc = "Generated from OpenAPI"
			}
			scope := module + ".api"
			if module == "core" {
				scope = "core.api"
			}
			entries = append(entries, capabilityEntry{
				CapabilityID:   id,
				Module:         module,
				Title:          title,
				Description:    desc,
				PermissionCode: permissionCodeFromID(id),
				AgentUsable:    boolPtr(false),
				RiskLevel:      riskFromMethod(method),
				Categories:     []string{module, "openapi", "generated"},
				Intents:        []string{module + "." + action},
				ToolScopes:     []string{scope},
				Policy:         capabilityPolicy{Prefer: "rest"},
				Docs:           []string{path},
				Protocols: []protocolEntry{{
					Channel:       "rest",
					Endpoint:      p,
					Method:        method,
					SchemaRef:     path + "#/paths/" + escapeJSONPointer(p) + "/" + strings.ToLower(method),
					AuthType:      auth,
					ActorContext:  inferActorContextFromPath(p),
					ResourceScope: "tenant",
					ToolScope:     scope,
				}},
			})
		}
	}
	return entries, nil
}

var (
	rePkg      = regexp.MustCompile(`(?m)^\s*package\s+([a-zA-Z0-9_.]+)\s*;`)
	reService  = regexp.MustCompile(`^\s*service\s+([A-Za-z0-9_]+)\s*\{`)
	reRPC      = regexp.MustCompile(`^\s*rpc\s+([A-Za-z0-9_]+)\s*\(`)
	reCloseObj = regexp.MustCompile(`^\s*}\s*;?\s*$`)
)

func genFromProto(path, prefix, auth string) ([]capabilityEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read proto %s: %w", path, err)
	}
	content := string(raw)
	pkg := "powerx.core.v1"
	if m := rePkg.FindStringSubmatch(content); len(m) == 2 {
		pkg = strings.TrimSpace(m[1])
	}
	module := moduleFromPackage(pkg)
	scope := module + ".grpc"

	lines := strings.Split(content, "\n")
	entries := make([]capabilityEntry, 0, 64)
	inService := false
	service := ""
	for _, line := range lines {
		if !inService {
			if m := reService.FindStringSubmatch(line); len(m) == 2 {
				inService = true
				service = m[1]
			}
			continue
		}
		if reCloseObj.MatchString(line) {
			inService = false
			service = ""
			continue
		}
		m := reRPC.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}
		rpc := m[1]
		action := slug(rpc)
		endpoint := pkg + "." + service
		id := joinID(prefix, "grpc", module, slug(service), action)
		title := service + "." + rpc
		entries = append(entries, capabilityEntry{
			CapabilityID:   id,
			Module:         module,
			Title:          title,
			Description:    "Generated from proto",
			PermissionCode: permissionCodeFromID(id),
			AgentUsable:    boolPtr(false),
			RiskLevel:      "medium",
			Categories:     []string{module, "grpc", "generated"},
			Intents:        []string{module + "." + action},
			ToolScopes:     []string{scope},
			Policy:         capabilityPolicy{Prefer: "grpc"},
			Docs:           []string{path},
			Protocols: []protocolEntry{{
				Channel:   "grpc",
				Endpoint:  endpoint,
				RPC:       rpc,
				SchemaRef: path + "#" + service,
				AuthType:  auth,
				ToolScope: scope,
			}},
		})
	}
	return entries, nil
}

func collectProtoFiles(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	files := make([]string, 0, 64)
	seen := map[string]struct{}{}
	for _, in := range inputs {
		if in == "" {
			continue
		}
		st, err := os.Stat(in)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", in, err)
		}
		if !st.IsDir() {
			if strings.HasSuffix(strings.ToLower(in), ".proto") {
				abs, _ := filepath.Abs(in)
				if _, ok := seen[abs]; !ok {
					seen[abs] = struct{}{}
					files = append(files, in)
				}
			}
			continue
		}
		err = filepath.WalkDir(in, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(path), ".proto") {
				return nil
			}
			abs, _ := filepath.Abs(path)
			if _, ok := seen[abs]; ok {
				return nil
			}
			seen[abs] = struct{}{}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

func collectGoFiles(inputs []string) ([]string, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	files := make([]string, 0, 128)
	seen := map[string]struct{}{}
	for _, in := range inputs {
		if in == "" {
			continue
		}
		st, err := os.Stat(in)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", in, err)
		}
		if !st.IsDir() {
			if strings.HasSuffix(strings.ToLower(in), ".go") {
				abs, _ := filepath.Abs(in)
				if _, ok := seen[abs]; !ok {
					seen[abs] = struct{}{}
					files = append(files, in)
				}
			}
			continue
		}
		err = filepath.WalkDir(in, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(path), ".go") {
				return nil
			}
			abs, _ := filepath.Abs(path)
			if _, ok := seen[abs]; ok {
				return nil
			}
			seen[abs] = struct{}{}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

var (
	reGroupAssign = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*:=\s*([A-Za-z_][A-Za-z0-9_]*)\.Group\("([^"]*)"\)`)
	reRouteCall   = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\.(GET|POST|PUT|PATCH|DELETE|OPTIONS|HEAD|Any)\("([^"]*)"`)
)

func genFromGinSource(path, prefix, auth, apiPrefix string, seenRoute map[string]struct{}) ([]capabilityEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read go source %s: %w", path, err)
	}
	lines := strings.Split(string(raw), "\n")
	groupBase := map[string]string{
		"publicGroup":    "",
		"public":         "",
		"protectedGroup": "",
		"protected":      "",
		"adminGroup":     "",
		"tenantGroup":    "/tenant",
		"group":          "",
		"g":              "",
	}
	out := make([]capabilityEntry, 0, 32)
	for _, line := range lines {
		if m := reGroupAssign.FindStringSubmatch(line); len(m) == 4 {
			child := strings.TrimSpace(m[1])
			parent := strings.TrimSpace(m[2])
			p := strings.TrimSpace(m[3])
			groupBase[child] = cleanPath(joinPath(groupBase[parent], p))
			continue
		}
		m := reRouteCall.FindStringSubmatch(line)
		if len(m) != 4 {
			continue
		}
		target := strings.TrimSpace(m[1])
		method := strings.ToUpper(strings.TrimSpace(m[2]))
		localPath := strings.TrimSpace(m[3])
		if method == "ANY" {
			continue
		}
		base := groupBase[target]
		joined := cleanPath(joinPath(base, localPath))
		full := joined
		// 部分路由组已包含 /api/v1 前缀，避免重复拼出 /api/v1/api/v1/*
		if !strings.HasPrefix(joined, "/api/") {
			full = cleanPath(joinPath(apiPrefix, joined))
		}
		if !strings.HasPrefix(full, "/api/") {
			continue
		}
		if generatedRouteExcluded(method, full) {
			continue
		}
		routeKey := method + " " + full
		if _, ok := seenRoute[routeKey]; ok {
			continue
		}
		module := moduleFromPath(full)
		action := slug(method + "_" + full)
		scope := module + ".api"
		if module == "core" {
			scope = "core.api"
		}
		id := joinID(prefix, "rest", module, "gin", action)
		out = append(out, capabilityEntry{
			CapabilityID:   id,
			Module:         module,
			Title:          method + " " + full,
			Description:    "Generated from Gin route source",
			PermissionCode: permissionCodeFromID(id),
			AgentUsable:    boolPtr(false),
			RiskLevel:      riskFromMethod(method),
			Categories:     []string{module, "gin", "generated"},
			Intents:        []string{module + "." + action},
			ToolScopes:     []string{scope},
			Policy:         capabilityPolicy{Prefer: "rest"},
			Docs:           []string{path},
			Protocols: []protocolEntry{{
				Channel:       "rest",
				Endpoint:      full,
				Method:        method,
				SchemaRef:     path + "#source",
				AuthType:      auth,
				ActorContext:  inferActorContextFromPath(full),
				ResourceScope: "tenant",
				ToolScope:     scope,
			}},
		})
	}
	return out, nil
}

func generatedRouteExcluded(method, path string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = cleanPath(path)
	switch method + " " + path {
	case "GET /api/v1/public/saas/registration-policy/effective",
		"POST /api/v1/public/saas/registration-requests":
		return true
	default:
		return false
	}
}

func joinPath(parts ...string) string {
	out := ""
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if out == "" {
			out = p
			continue
		}
		out = strings.TrimRight(out, "/") + "/" + strings.TrimLeft(p, "/")
	}
	return out
}

func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return p
}

func moduleFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "api" {
			if i+1 < len(parts) && strings.HasPrefix(parts[i+1], "v") {
				i++
			}
			if i+1 < len(parts) {
				n := slug(parts[i+1])
				if n != "" {
					return n
				}
			}
		}
	}
	for _, p := range parts {
		n := slug(p)
		if n != "" {
			return n
		}
	}
	return "core"
}

func inferActorContextFromPath(path string) string {
	path = cleanPath(path)
	if path == "/api/v1/admin" || strings.HasPrefix(path, "/api/v1/admin/") {
		return "admin_user"
	}
	if path == "/api/v1/customer" || strings.HasPrefix(path, "/api/v1/customer/") {
		return "customer_actor"
	}
	return "service_actor"
}

func permissionCodeFromID(id string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(id), "com.corex.")
	trimmed = strings.ReplaceAll(trimmed, "/", ".")
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '.'
	})
	if len(parts) == 0 {
		return "corex.generated:use"
	}
	module := slug(parts[0])
	action := "use"
	if len(parts) > 1 {
		action = slug(parts[len(parts)-1])
	}
	if module == "" {
		module = "generated"
	}
	if action == "" {
		action = "use"
	}
	return "corex." + module + ":" + action
}

func riskFromMethod(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD", "OPTIONS":
		return "low"
	case "DELETE":
		return "high"
	default:
		return "medium"
	}
}

func boolPtr(v bool) *bool {
	return &v
}

func moduleFromPackage(pkg string) string {
	parts := strings.Split(strings.TrimSpace(pkg), ".")
	if len(parts) >= 2 {
		m := slug(parts[1])
		if m != "" {
			return m
		}
	}
	if len(parts) > 0 {
		m := slug(parts[0])
		if m != "" {
			return m
		}
	}
	return "core"
}

func joinID(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(strings.TrimSpace(p), ".")
		if p == "" {
			continue
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return "generated.capability"
	}
	return strings.Join(clean, ".")
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(in string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	if in == "" {
		return ""
	}
	in = strings.ReplaceAll(in, "{", "")
	in = strings.ReplaceAll(in, "}", "")
	in = strings.ReplaceAll(in, ":", "_")
	in = slugRe.ReplaceAllString(in, "_")
	in = strings.Trim(in, "_")
	for strings.Contains(in, "__") {
		in = strings.ReplaceAll(in, "__", "_")
	}
	return in
}

func escapeJSONPointer(path string) string {
	p := strings.TrimPrefix(path, "/")
	p = strings.ReplaceAll(p, "~", "~0")
	p = strings.ReplaceAll(p, "/", "~1")
	return p
}

func fatalf(format string, args ...interface{}) {
	logger.ErrorF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), format, args...)
	os.Exit(1)
}

func ensure(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func check(errs ...error) error {
	list := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			list = append(list, err)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return errors.Join(list...)
}
