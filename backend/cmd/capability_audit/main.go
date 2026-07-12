package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

type config struct {
	RepoRoot        string
	PlatformDir     string
	CandidateFiles  []string
	IgnoreFiles     []string
	RequiredFiles   []string
	ScanRoots       []string
	RouteScanRoots  []string
	RequiredIDs     []string
	Fix             bool
	FixOutput       string
	IncludeDocs     bool
	FailOnDraftOnly bool
	CheckRoutes     bool
}

type capabilityFile struct {
	Version      int               `yaml:"version"`
	Capabilities []capabilityEntry `yaml:"capabilities"`
}

type capabilityEntry struct {
	CapabilityID   string            `yaml:"capability_id"`
	Module         string            `yaml:"module"`
	Title          string            `yaml:"title"`
	Description    string            `yaml:"description,omitempty"`
	PermissionCode string            `yaml:"permission_code,omitempty"`
	AgentUsable    *bool             `yaml:"agent_usable,omitempty"`
	RiskLevel      string            `yaml:"risk_level,omitempty"`
	Categories     []string          `yaml:"categories,omitempty"`
	Intents        []string          `yaml:"intents,omitempty"`
	ToolScopes     []string          `yaml:"tool_scopes,omitempty"`
	Policy         map[string]any    `yaml:"policy,omitempty"`
	Docs           []string          `yaml:"docs,omitempty"`
	Protocols      []protocolBinding `yaml:"protocols,omitempty"`
}

type protocolBinding struct {
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

type requiredFile struct {
	RequiredCapabilities []requiredCapability `yaml:"required_capabilities"`
}

type ignoreFile struct {
	Routes []ignoreRoute `yaml:"routes"`
}

type ignoreRoute struct {
	Method   string `yaml:"method"`
	Path     string `yaml:"path"`
	Category string `yaml:"category"`
	Reason   string `yaml:"reason"`
}

type requiredCapability struct {
	CapabilityID string `yaml:"capability_id"`
	Reason       string `yaml:"reason"`
	Source       string `yaml:"source"`
}

type reference struct {
	ID     string
	Source string
	Line   int
	Kind   string
}

type routeRef struct {
	Method string
	Path   string
	Source string
	Line   int
}

type declaredCapabilities struct {
	IDs        map[string]struct{}
	RESTRoutes []routeRef
}

func (d *declaredCapabilities) merge(other declaredCapabilities) {
	if d.IDs == nil {
		d.IDs = map[string]struct{}{}
	}
	for id := range other.IDs {
		d.IDs[id] = struct{}{}
	}
	d.RESTRoutes = append(d.RESTRoutes, other.RESTRoutes...)
}

var capabilityIDPattern = regexp.MustCompile(`com\.corex\.[A-Za-z0-9][A-Za-z0-9._/-]*[A-Za-z0-9]`)

var (
	reGroupAssign = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*:=\s*([A-Za-z_][A-Za-z0-9_]*)\.Group\("([^"]*)"\)`)
	reRouteCall   = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\.(GET|POST|PUT|PATCH|DELETE|OPTIONS|HEAD|Any)\("([^"]*)"`)
)

func main() {
	var scanRoots stringList
	var routeScanRoots stringList
	var candidateFiles stringList
	var ignoreFiles stringList
	var requiredFiles stringList
	var requiredIDs stringList
	cfg := config{}
	flag.StringVar(&cfg.RepoRoot, "repo-root", ".", "repository root")
	flag.StringVar(&cfg.PlatformDir, "platform-dir", "backend/config/platform_capabilities", "platform capability yaml directory")
	flag.Var(&candidateFiles, "candidate-file", "generated candidate capability yaml file; repeatable or comma-separated")
	flag.Var(&ignoreFiles, "ignore-file", "yaml file with explicit capability audit ignore rules; repeatable or comma-separated")
	flag.Var(&scanRoots, "scan", "file or directory to scan for com.corex capability references; repeatable or comma-separated")
	flag.Var(&routeScanRoots, "route-scan", "Go file or directory to scan for Gin HTTP routes; repeatable or comma-separated")
	flag.Var(&requiredFiles, "required-file", "yaml file with required_capabilities; repeatable or comma-separated")
	flag.Var(&requiredIDs, "required", "required capability id; repeatable or comma-separated")
	flag.BoolVar(&cfg.Fix, "fix", envBool("CAPABILITY_AUDIT_FIX"), "write a draft yaml for missing declarations")
	flag.StringVar(&cfg.FixOutput, "fix-output", "tmp/capability-audit/missing.platform-capabilities.yaml", "draft yaml output path")
	flag.BoolVar(&cfg.IncludeDocs, "include-docs", false, "scan docs/specs paths")
	flag.BoolVar(&cfg.CheckRoutes, "check-routes", true, "verify Gin HTTP routes are covered by REST capability protocols")
	flag.Parse()

	cfg.ScanRoots = append(splitEnv("CAPABILITY_AUDIT_SCAN"), scanRoots...)
	cfg.RouteScanRoots = append(splitEnv("CAPABILITY_AUDIT_ROUTE_SCAN"), routeScanRoots...)
	cfg.CandidateFiles = append(splitEnv("CAPABILITY_AUDIT_CANDIDATE_FILE"), candidateFiles...)
	cfg.IgnoreFiles = append(splitEnv("CAPABILITY_AUDIT_IGNORE_FILE"), ignoreFiles...)
	cfg.RequiredFiles = append(splitEnv("CAPABILITY_AUDIT_REQUIRED_FILE"), requiredFiles...)
	cfg.RequiredIDs = append(splitEnv("CAPABILITY_AUDIT_REQUIRED"), requiredIDs...)

	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	repoRoot, err := filepath.Abs(cfg.RepoRoot)
	if err != nil {
		return err
	}
	platformDir := resolvePath(repoRoot, cfg.PlatformDir)
	declared, err := loadDeclaredCapabilities(platformDir)
	if err != nil {
		return err
	}
	candidates, err := loadCandidateCapabilityFiles(repoRoot, cfg.CandidateFiles)
	if err != nil {
		return err
	}
	ignores, err := loadIgnoreRules(repoRoot, cfg.IgnoreFiles)
	if err != nil {
		return err
	}
	refs, err := collectReferences(repoRoot, cfg)
	if err != nil {
		return err
	}

	missing := missingReferences(refs, declared.IDs)
	uncoveredRoutes, err := collectUncoveredRoutes(repoRoot, cfg, declared.RESTRoutes, ignores.Routes)
	if err != nil {
		return err
	}
	if len(missing) == 0 && len(uncoveredRoutes) == 0 {
		fmt.Printf("capability-audit: ok, declared=%d referenced=%d rest_routes=%d candidates=%d ignored_route_rules=%d\n", len(declared.IDs), len(uniqueReferenceIDs(refs)), len(declared.RESTRoutes), len(candidates.IDs), len(ignores.Routes))
		return nil
	}

	if len(missing) > 0 {
		printMissing(missing)
	}
	if len(uncoveredRoutes) > 0 {
		printUncoveredRoutes(uncoveredRoutes)
	}
	if cfg.Fix {
		if err := writeDraft(repoRoot, cfg.FixOutput, missing, uncoveredRoutes); err != nil {
			return err
		}
		fmt.Printf("capability-audit: draft written to %s\n", resolvePath(repoRoot, cfg.FixOutput))
	}
	return fmt.Errorf("capability-audit: %d missing platform capability declaration(s), %d uncovered HTTP route(s)", len(missing), len(uncoveredRoutes))
}

func loadDeclaredCapabilities(dir string) (declaredCapabilities, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return declaredCapabilities{}, fmt.Errorf("read platform capability dir: %w", err)
	}
	declared := map[string]struct{}{}
	var restRoutes []routeRef
	var errs []error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".yaml") && !strings.HasSuffix(strings.ToLower(name), ".yml") {
			continue
		}
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		var file capabilityFile
		if err := yaml.Unmarshal(raw, &file); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		for _, cap := range file.Capabilities {
			id := strings.TrimSpace(cap.CapabilityID)
			if id == "" {
				continue
			}
			if err := validateCapabilityMetadata(path, cap); err != nil {
				errs = append(errs, err)
			}
			if _, exists := declared[id]; exists {
				errs = append(errs, fmt.Errorf("duplicate capability declaration: %s", id))
			}
			declared[id] = struct{}{}
			for _, protocol := range cap.Protocols {
				if !strings.EqualFold(strings.TrimSpace(protocol.Channel), "rest") {
					continue
				}
				if err := validateRESTProtocolMetadata(path, id, protocol); err != nil {
					errs = append(errs, err)
				}
				method := strings.ToUpper(strings.TrimSpace(protocol.Method))
				endpoint := cleanRoutePath(protocol.Endpoint)
				if method == "" || endpoint == "" || method == "TODO" || strings.Contains(endpoint, "TODO") {
					continue
				}
				restRoutes = append(restRoutes, routeRef{
					Method: method,
					Path:   endpoint,
					Source: filepath.ToSlash(path),
				})
			}
		}
	}
	if len(errs) > 0 {
		return declaredCapabilities{}, errors.Join(errs...)
	}
	return declaredCapabilities{IDs: declared, RESTRoutes: restRoutes}, nil
}

func loadCandidateCapabilityFiles(repoRoot string, files []string) (declaredCapabilities, error) {
	out := declaredCapabilities{IDs: map[string]struct{}{}}
	var errs []error
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		path := resolvePath(repoRoot, file)
		declared, err := parseDeclaredCapabilityFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out.merge(declared)
	}
	if len(errs) > 0 {
		return declaredCapabilities{}, errors.Join(errs...)
	}
	return out, nil
}

func parseDeclaredCapabilityFile(path string) (declaredCapabilities, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return declaredCapabilities{IDs: map[string]struct{}{}}, nil
		}
		return declaredCapabilities{}, fmt.Errorf("%s: %w", path, err)
	}
	var file capabilityFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return declaredCapabilities{}, fmt.Errorf("%s: %w", path, err)
	}
	out := declaredCapabilities{IDs: map[string]struct{}{}}
	for _, cap := range file.Capabilities {
		id := strings.TrimSpace(cap.CapabilityID)
		if id != "" {
			out.IDs[id] = struct{}{}
		}
		for _, protocol := range cap.Protocols {
			if !strings.EqualFold(strings.TrimSpace(protocol.Channel), "rest") {
				continue
			}
			method := strings.ToUpper(strings.TrimSpace(protocol.Method))
			endpoint := cleanRoutePath(protocol.Endpoint)
			if method == "" || endpoint == "" || method == "TODO" || strings.Contains(endpoint, "TODO") {
				continue
			}
			out.RESTRoutes = append(out.RESTRoutes, routeRef{
				Method: method,
				Path:   endpoint,
				Source: filepath.ToSlash(path),
			})
		}
	}
	return out, nil
}

type ignoreRules struct {
	Routes []ignoreRoute
}

func loadIgnoreRules(repoRoot string, files []string) (ignoreRules, error) {
	if len(files) == 0 {
		files = []string{"backend/config/capability_audit_ignore.yaml"}
	}
	var out ignoreRules
	var errs []error
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file == "" {
			continue
		}
		path := resolvePath(repoRoot, file)
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = append(errs, fmt.Errorf("read ignore file %s: %w", path, err))
			continue
		}
		var parsed ignoreFile
		if err := yaml.Unmarshal(raw, &parsed); err != nil {
			errs = append(errs, fmt.Errorf("parse ignore file %s: %w", path, err))
			continue
		}
		for i, route := range parsed.Routes {
			route.Method = strings.ToUpper(strings.TrimSpace(route.Method))
			route.Path = cleanRoutePath(route.Path)
			route.Category = strings.TrimSpace(route.Category)
			route.Reason = strings.TrimSpace(route.Reason)
			if route.Method == "" || route.Path == "" || route.Category == "" || route.Reason == "" {
				errs = append(errs, fmt.Errorf("%s routes[%d]: method, path, category and reason are required", path, i))
				continue
			}
			out.Routes = append(out.Routes, route)
		}
	}
	if len(errs) > 0 {
		return ignoreRules{}, errors.Join(errs...)
	}
	return out, nil
}

func validateRESTProtocolMetadata(path string, capabilityID string, protocol protocolBinding) error {
	endpoint := cleanRoutePath(protocol.Endpoint)
	actor := strings.TrimSpace(protocol.ActorContext)
	scope := strings.TrimSpace(protocol.ResourceScope)
	var errs []error
	if actor == "" {
		errs = append(errs, fmt.Errorf("%s: capability %s REST %s %s missing actor_context", path, capabilityID, strings.ToUpper(strings.TrimSpace(protocol.Method)), endpoint))
	}
	if scope == "" {
		errs = append(errs, fmt.Errorf("%s: capability %s REST %s %s missing resource_scope", path, capabilityID, strings.ToUpper(strings.TrimSpace(protocol.Method)), endpoint))
	}
	if protocol.STSDirect && !strings.EqualFold(actor, "service_actor") {
		errs = append(errs, fmt.Errorf("%s: capability %s REST %s %s has sts_direct=true but actor_context is %q", path, capabilityID, strings.ToUpper(strings.TrimSpace(protocol.Method)), endpoint, actor))
	}
	if protocol.STSDirect && (endpoint == "/api/v1/admin" || strings.HasPrefix(endpoint, "/api/v1/admin/") || endpoint == "/admin" || strings.HasPrefix(endpoint, "/admin/")) {
		errs = append(errs, fmt.Errorf("%s: capability %s REST %s %s must not expose admin endpoint through STS direct", path, capabilityID, strings.ToUpper(strings.TrimSpace(protocol.Method)), endpoint))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func validateCapabilityMetadata(path string, cap capabilityEntry) error {
	id := strings.TrimSpace(cap.CapabilityID)
	var errs []error
	if strings.TrimSpace(cap.PermissionCode) == "" {
		errs = append(errs, fmt.Errorf("%s: capability %s missing permission_code", path, id))
	}
	if cap.AgentUsable == nil {
		errs = append(errs, fmt.Errorf("%s: capability %s missing agent_usable", path, id))
	}
	switch strings.TrimSpace(cap.RiskLevel) {
	case "low", "medium", "high", "critical":
	default:
		errs = append(errs, fmt.Errorf("%s: capability %s has invalid or missing risk_level", path, id))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func collectReferences(repoRoot string, cfg config) ([]reference, error) {
	var refs []reference
	for _, id := range cfg.RequiredIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			refs = append(refs, reference{ID: id, Kind: "required", Source: "CAPABILITY_AUDIT_REQUIRED"})
		}
	}
	for _, file := range cfg.RequiredFiles {
		fileRefs, err := loadRequiredFile(resolvePath(repoRoot, file))
		if err != nil {
			return nil, err
		}
		refs = append(refs, fileRefs...)
	}
	scanRoots := cfg.ScanRoots
	if len(scanRoots) == 0 {
		scanRoots = []string{"backend/internal", "backend/config", "config", "powerx-plugin"}
	}
	for _, root := range scanRoots {
		scanned, err := scanRoot(repoRoot, resolvePath(repoRoot, root), cfg.IncludeDocs)
		if err != nil {
			return nil, err
		}
		refs = append(refs, scanned...)
	}
	return refs, nil
}

func loadRequiredFile(path string) ([]reference, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read required capability file %s: %w", path, err)
	}
	var file requiredFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse required capability file %s: %w", path, err)
	}
	refs := make([]reference, 0, len(file.RequiredCapabilities))
	for _, item := range file.RequiredCapabilities {
		id := strings.TrimSpace(item.CapabilityID)
		if id == "" {
			continue
		}
		source := strings.TrimSpace(item.Source)
		if source == "" {
			source = path
		}
		refs = append(refs, reference{ID: id, Source: source, Kind: "required"})
	}
	return refs, nil
}

func scanRoot(repoRoot, root string, includeDocs bool) ([]reference, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	if !info.IsDir() {
		files = append(files, root)
	} else {
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if shouldSkipDir(repoRoot, path, includeDocs) {
					return filepath.SkipDir
				}
				return nil
			}
			if shouldScanFile(path) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	var refs []reference
	for _, file := range files {
		fileRefs, err := scanFile(repoRoot, file)
		if err != nil {
			return nil, err
		}
		refs = append(refs, fileRefs...)
	}
	return refs, nil
}

func shouldSkipDir(repoRoot, path string, includeDocs bool) bool {
	rel := slashRel(repoRoot, path)
	if rel == "." {
		return false
	}
	base := filepath.Base(path)
	switch base {
	case ".git", "node_modules", "dist", ".output", ".nuxt", ".cache", "vendor":
		return true
	}
	if strings.HasPrefix(rel, "backend/config/platform_capabilities") {
		return true
	}
	if !includeDocs && (strings.HasPrefix(rel, "docs") || strings.HasPrefix(rel, "specs")) {
		return true
	}
	return false
}

func shouldScanFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".yaml", ".yml", ".json", ".toml", ".md", ".ts", ".tsx", ".vue":
		return true
	default:
		return false
	}
}

func scanFile(repoRoot, path string) ([]reference, error) {
	if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(filepath.ToSlash(path), "backend/config/capability_audit_required.yaml") {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var refs []reference
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		for _, match := range capabilityIDPattern.FindAllString(scanner.Text(), -1) {
			id := strings.Trim(match, "`'\"),]}>")
			if !isConcreteCapabilityID(id) {
				continue
			}
			refs = append(refs, reference{ID: id, Source: slashRel(repoRoot, path), Line: lineNo, Kind: "scan"})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return refs, nil
}

func isConcreteCapabilityID(id string) bool {
	trimmed := strings.TrimPrefix(strings.TrimSpace(id), "com.corex.")
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '.' || r == '/'
	})
	return len(parts) >= 2
}

func missingReferences(refs []reference, declared map[string]struct{}) map[string][]reference {
	out := map[string][]reference{}
	for _, ref := range refs {
		if ref.ID == "" {
			continue
		}
		if _, ok := declared[ref.ID]; ok {
			continue
		}
		out[ref.ID] = append(out[ref.ID], ref)
	}
	return out
}

func collectUncoveredRoutes(repoRoot string, cfg config, declaredRoutes []routeRef, ignoredRoutes []ignoreRoute) ([]routeRef, error) {
	if !cfg.CheckRoutes {
		return nil, nil
	}
	roots := cfg.RouteScanRoots
	if len(roots) == 0 {
		roots = []string{"backend/internal/transport/http/admin", "backend/internal/transport/http/openapi"}
	}
	var actual []routeRef
	for _, root := range roots {
		files, err := collectGoFiles(resolvePath(repoRoot, root))
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			routes, err := scanGinRoutes(repoRoot, file)
			if err != nil {
				return nil, err
			}
			actual = append(actual, routes...)
		}
	}
	seen := map[string]routeRef{}
	for _, route := range actual {
		if shouldIgnoreRoute(route) {
			continue
		}
		key := route.Method + " " + route.Path
		if _, exists := seen[key]; !exists {
			seen[key] = route
		}
	}
	var missing []routeRef
	for _, route := range seen {
		if !routeCovered(route, declaredRoutes) {
			if routeIgnored(route, ignoredRoutes) {
				continue
			}
			missing = append(missing, route)
		}
	}
	sort.Slice(missing, func(i, j int) bool {
		if missing[i].Path == missing[j].Path {
			return missing[i].Method < missing[j].Method
		}
		return missing[i].Path < missing[j].Path
	})
	return missing, nil
}

func routeIgnored(actual routeRef, ignored []ignoreRoute) bool {
	for _, candidate := range ignored {
		if candidate.Method != "*" && !strings.EqualFold(actual.Method, candidate.Method) {
			continue
		}
		if ignorePathMatches(actual.Path, candidate.Path) {
			return true
		}
	}
	return false
}

func ignorePathMatches(actual, pattern string) bool {
	actual = cleanRoutePath(actual)
	pattern = cleanRoutePath(pattern)
	if pattern == actual {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return actual == prefix || strings.HasPrefix(actual, prefix+"/")
	}
	return routePatternMatches(actual, pattern)
}

func collectGoFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		if strings.HasSuffix(root, ".go") && !strings.HasSuffix(root, "_test.go") {
			return []string{root}, nil
		}
		return nil, nil
	}
	var files []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func scanGinRoutes(repoRoot, path string) ([]routeRef, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
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
	lines := strings.Split(string(raw), "\n")
	var routes []routeRef
	for i, line := range lines {
		if m := reGroupAssign.FindStringSubmatch(line); len(m) == 4 {
			child := strings.TrimSpace(m[1])
			parent := strings.TrimSpace(m[2])
			local := strings.TrimSpace(m[3])
			groupBase[child] = cleanRoutePath(joinRoutePath(groupBase[parent], local))
			continue
		}
		m := reRouteCall.FindStringSubmatch(line)
		if len(m) != 4 {
			continue
		}
		method := strings.ToUpper(strings.TrimSpace(m[2]))
		if method == "ANY" {
			continue
		}
		target := strings.TrimSpace(m[1])
		local := strings.TrimSpace(m[3])
		full := cleanRoutePath(joinRoutePath(groupBase[target], local))
		if !strings.HasPrefix(full, "/api/") {
			full = cleanRoutePath(joinRoutePath("/api/v1", full))
		}
		routes = append(routes, routeRef{
			Method: method,
			Path:   full,
			Source: slashRel(repoRoot, path),
			Line:   i + 1,
		})
	}
	return routes, nil
}

func routeCovered(actual routeRef, declared []routeRef) bool {
	for _, candidate := range declared {
		if !strings.EqualFold(actual.Method, candidate.Method) {
			continue
		}
		if routePatternMatches(actual.Path, candidate.Path) {
			return true
		}
	}
	return false
}

func routePatternMatches(actual, declared string) bool {
	a := strings.Split(strings.Trim(cleanRoutePath(actual), "/"), "/")
	d := strings.Split(strings.Trim(cleanRoutePath(declared), "/"), "/")
	if len(a) != len(d) {
		return false
	}
	for i := range a {
		if isRouteParam(a[i]) || isRouteParam(d[i]) {
			continue
		}
		if a[i] != d[i] {
			return false
		}
	}
	return true
}

func isRouteParam(part string) bool {
	return strings.HasPrefix(part, ":") || (strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}"))
}

func shouldIgnoreRoute(route routeRef) bool {
	path := cleanRoutePath(route.Path)
	if path == "/api/v1/health" || path == "/api/v1/healthz" {
		return true
	}
	if strings.Contains(path, "/swagger/") {
		return true
	}
	// Public auth/user bootstrap routes are identity entrypoints, not tenant capabilities.
	if strings.HasPrefix(path, "/api/v1/auth/") {
		return true
	}
	return false
}

func printUncoveredRoutes(routes []routeRef) {
	fmt.Fprintln(os.Stderr, "capability-audit: uncovered HTTP routes:")
	limit := len(routes)
	for i := 0; i < limit; i++ {
		line := ""
		if routes[i].Line > 0 {
			line = fmt.Sprintf(":%d", routes[i].Line)
		}
		fmt.Fprintf(os.Stderr, "- %s %s\n  source: %s%s\n", routes[i].Method, routes[i].Path, routes[i].Source, line)
	}
}

func uniqueReferenceIDs(refs []reference) map[string]struct{} {
	out := map[string]struct{}{}
	for _, ref := range refs {
		if ref.ID != "" {
			out[ref.ID] = struct{}{}
		}
	}
	return out
}

func printMissing(missing map[string][]reference) {
	ids := sortedKeys(missing)
	fmt.Fprintln(os.Stderr, "capability-audit: missing platform capability declarations:")
	for _, id := range ids {
		fmt.Fprintf(os.Stderr, "- %s\n", id)
		refs := missing[id]
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].Source == refs[j].Source {
				return refs[i].Line < refs[j].Line
			}
			return refs[i].Source < refs[j].Source
		})
		limit := len(refs)
		if limit > 5 {
			limit = 5
		}
		for i := 0; i < limit; i++ {
			line := ""
			if refs[i].Line > 0 {
				line = fmt.Sprintf(":%d", refs[i].Line)
			}
			fmt.Fprintf(os.Stderr, "  source: %s%s (%s)\n", refs[i].Source, line, refs[i].Kind)
		}
		if len(refs) > limit {
			fmt.Fprintf(os.Stderr, "  ... %d more source(s)\n", len(refs)-limit)
		}
	}
}

func writeDraft(repoRoot, output string, missing map[string][]reference, uncoveredRoutes []routeRef) error {
	ids := sortedKeys(missing)
	file := capabilityFile{Version: 1, Capabilities: make([]capabilityEntry, 0, len(ids)+len(uncoveredRoutes))}
	usable := false
	seen := map[string]struct{}{}
	for _, id := range ids {
		module := inferModule(id)
		scope := module
		if scope == "" {
			scope = "corex"
		}
		entry := capabilityEntry{
			CapabilityID:   id,
			Module:         module,
			Title:          titleFromID(id),
			Description:    "TODO: fill the business description before moving this draft into backend/config/platform_capabilities.",
			PermissionCode: permissionCodeFromID(id),
			AgentUsable:    &usable,
			RiskLevel:      "medium",
			Categories:     []string{module},
			Intents:        []string{intentFromID(id)},
			ToolScopes:     []string{scope},
			Policy:         map[string]any{"prefer": "rest"},
			Docs:           []string{"TODO: add contract or feature guide path"},
			Protocols: []protocolBinding{{
				Channel:       "rest",
				Endpoint:      "TODO: add real /api/v1 endpoint",
				Method:        "TODO",
				AuthType:      "tenant_jwt",
				ActorContext:  "TODO",
				ResourceScope: "TODO",
				ToolScope:     scope,
			}},
		}
		file.Capabilities = append(file.Capabilities, entry)
		seen[id] = struct{}{}
	}
	for _, route := range uncoveredRoutes {
		entry := draftEntryFromRoute(route)
		if _, exists := seen[entry.CapabilityID]; exists {
			continue
		}
		file.Capabilities = append(file.Capabilities, entry)
		seen[entry.CapabilityID] = struct{}{}
	}
	raw, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString("# Generated by capability-audit. This is a draft, not an active platform capability file.\n")
	buf.WriteString("# Review endpoint/protocol/permission_code before moving entries into backend/config/platform_capabilities.\n")
	buf.Write(raw)
	path := resolvePath(repoRoot, output)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func inferModule(id string) string {
	trimmed := strings.TrimPrefix(id, "com.corex.")
	for _, prefix := range []string{"grpc.", "rest."} {
		trimmed = strings.TrimPrefix(trimmed, prefix)
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '.' || r == '/' || r == '_' || r == '-'
	})
	if len(parts) == 0 {
		return "corex"
	}
	return strings.ToLower(parts[0])
}

func titleFromID(id string) string {
	trimmed := strings.TrimPrefix(id, "com.corex.")
	trimmed = strings.ReplaceAll(trimmed, ".", " ")
	trimmed = strings.ReplaceAll(trimmed, "/", " ")
	trimmed = strings.ReplaceAll(trimmed, "_", " ")
	trimmed = strings.ReplaceAll(trimmed, "-", " ")
	words := strings.Fields(trimmed)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	if len(words) == 0 {
		return id
	}
	return strings.Join(words, " ")
}

func permissionCodeFromID(id string) string {
	module := inferModule(id)
	action := "use"
	parts := strings.FieldsFunc(id, func(r rune) bool {
		return r == '.' || r == '/'
	})
	if len(parts) > 0 {
		action = strings.ToLower(parts[len(parts)-1])
	}
	if module == "" {
		module = "corex"
	}
	return fmt.Sprintf("corex.%s:%s", module, action)
}

func intentFromID(id string) string {
	trimmed := strings.TrimPrefix(id, "com.corex.")
	trimmed = strings.ReplaceAll(trimmed, "/", ".")
	trimmed = strings.ReplaceAll(trimmed, "-", ".")
	return strings.ToLower(trimmed)
}

func draftEntryFromRoute(route routeRef) capabilityEntry {
	module := moduleFromRoute(route.Path)
	scope := module + ".api"
	action := slug(strings.ToLower(route.Method) + "_" + route.Path)
	id := "com.corex.rest." + module + ".gin." + action
	return capabilityEntry{
		CapabilityID:   id,
		Module:         module,
		Title:          route.Method + " " + route.Path,
		Description:    "TODO: review business semantics before moving this draft into backend/config/platform_capabilities.",
		PermissionCode: "corex." + module + ":" + actionFromMethod(route.Method),
		AgentUsable:    boolPtr(false),
		RiskLevel:      riskFromMethod(route.Method),
		Categories:     []string{module, "rest", "generated"},
		Intents:        []string{module + "." + action},
		ToolScopes:     []string{scope},
		Policy:         map[string]any{"prefer": "rest"},
		Docs:           []string{route.Source},
		Protocols: []protocolBinding{{
			Channel:       "rest",
			Endpoint:      route.Path,
			Method:        route.Method,
			AuthType:      "tenant_jwt",
			ActorContext:  inferActorContextFromRoute(route.Path),
			ResourceScope: "tenant",
			ToolScope:     scope,
		}},
	}
}

func inferActorContextFromRoute(path string) string {
	path = cleanRoutePath(path)
	if path == "/api/v1/admin" || strings.HasPrefix(path, "/api/v1/admin/") {
		return "admin_user"
	}
	if path == "/api/v1/customer" || strings.HasPrefix(path, "/api/v1/customer/") {
		return "customer_actor"
	}
	return "service_actor"
}

func moduleFromRoute(path string) string {
	parts := strings.Split(strings.Trim(cleanRoutePath(path), "/"), "/")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "api" || part == "v1" || part == "admin" || part == "tenant" || part == "internal" || part == "public" {
			continue
		}
		if isRouteParam(part) {
			continue
		}
		filtered = append(filtered, strings.ReplaceAll(part, "-", "_"))
	}
	if len(filtered) == 0 {
		return "core"
	}
	return strings.ToLower(filtered[0])
}

func actionFromMethod(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD":
		return "read"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return "use"
	}
}

func riskFromMethod(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD":
		return "low"
	default:
		return "medium"
	}
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDot := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDot = false
			continue
		}
		if !lastDot {
			b.WriteByte('.')
			lastDot = true
		}
	}
	return strings.Trim(b.String(), ".")
}

func boolPtr(value bool) *bool {
	return &value
}

func joinRoutePath(parts ...string) string {
	out := ""
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if out == "" {
			out = part
			continue
		}
		out = strings.TrimRight(out, "/") + "/" + strings.TrimLeft(part, "/")
	}
	return cleanRoutePath(out)
}

func cleanRoutePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, "\\", "/")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func resolvePath(repoRoot, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(repoRoot, path)
}

func slashRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func splitEnv(name string) []string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func envBool(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes"
}
