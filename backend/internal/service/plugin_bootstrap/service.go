package plugin_bootstrap

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	auditpkg "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"golang.org/x/mod/semver"
	"gorm.io/datatypes"
)

var pluginIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

// Options configure the bootstrap service.
type Options struct {
	TemplatesPath   string
	DefaultTemplate string
	AllowHosts      []string
	Auditor         auditpkg.Auditor
	AuditSvc        auditpkg.Service
	Now             func() time.Time
}

// Service validates scaffold requests and doctor payloads.
type Service struct {
	registry *templateRegistry
	opts     Options
	now      func() time.Time
}

// NewService loads the template index and returns a service instance.
func NewService(opts Options) (*Service, error) {
	if strings.TrimSpace(opts.TemplatesPath) == "" {
		return nil, errors.New("templates path is required")
	}
	reg, err := newTemplateRegistry(opts.TemplatesPath)
	if err != nil {
		return nil, fmt.Errorf("load template index: %w", err)
	}
	if opts.Auditor == nil {
		opts.Auditor = auditpkg.Noop{}
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		registry: reg,
		opts:     opts,
		now:      now,
	}, nil
}

// ListTemplates returns sanitized template metadata.
func (s *Service) ListTemplates(_ context.Context) []TemplateSummary {
	raw := s.registry.list()
	summaries := make([]TemplateSummary, 0, len(raw))
	for _, spec := range raw {
		summaries = append(summaries, spec.CloneSummary())
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		return summaries[i].ID < summaries[j].ID
	})
	return summaries
}

// ValidateBootstrap validates scaffold metadata and returns actionable feedback.
func (s *Service) ValidateBootstrap(ctx context.Context, input BootstrapValidateInput) (*BootstrapValidateResult, error) {
	spec, err := s.resolveTemplate(strings.TrimSpace(input.TemplateID))
	if err != nil {
		return nil, err
	}

	pluginID := strings.TrimSpace(input.PluginID)
	if pluginID == "" {
		return nil, errors.New("pluginId is required")
	}

	issues := make([]BootstrapIssue, 0)
	if !pluginIDPattern.MatchString(pluginID) {
		issues = append(issues, BootstrapIssue{
			Code:     "PLUGIN_ID_INVALID",
			Severity: "error",
			Message:  "pluginId must be lowercase alphanumeric and may include '.' or '-'",
			Hint:     "Example: com.powerx.analytics or demo-plugin",
		})
	}

	if !compareVersions(input.CLIVersion, spec.CLI.MinVersion) {
		issues = append(issues, BootstrapIssue{
			Code:     "CLI_VERSION_OUTDATED",
			Severity: "error",
			Message:  fmt.Sprintf("CLI version %s is below template requirement %s", safeVersion(input.CLIVersion), spec.CLI.MinVersion),
			Hint:     "Upgrade px-plugin CLI via `go install` or download the latest binary from PowerX release page.",
		})
	}

	if host := strings.ToLower(strings.TrimSpace(input.GitHost)); host != "" && !containsString(s.opts.AllowHosts, host) {
		issues = append(issues, BootstrapIssue{
			Code:     "GIT_HOST_UNVERIFIED",
			Severity: "warning",
			Message:  fmt.Sprintf("Git host %s not found in allowlist", host),
			Hint:     "Set --git-host to an approved domain or update allowlist in plugin_bootstrap config.",
		})
	}

	modulePath := strings.TrimSpace(input.ModulePath)
	if modulePath == "" {
		modulePath = defaultModulePath(pluginID)
	}

	status := "ready"
	for _, issue := range issues {
		if issue.Severity == "error" {
			status = "blocked"
			break
		}
	}

	result := &BootstrapValidateResult{
		Status:          status,
		PluginID:        pluginID,
		ModulePath:      modulePath,
		Template:        spec.CloneSummary(),
		Issues:          issues,
		Recommendations: append([]string(nil), spec.Recommended...),
	}

	s.emitAudit(ctx, "PLUGIN_BOOTSTRAP_VALIDATE", result.Status, pluginID, map[string]any{
		"template_id": spec.ID,
		"issues":      issues,
	})

	return result, nil
}

// CheckEnvironment validates local toolchain versions / binaries.
func (s *Service) CheckEnvironment(ctx context.Context, input EnvironmentCheckInput) (*EnvironmentCheckReport, error) {
	spec, err := s.resolveTemplate(strings.TrimSpace(input.TemplateID))
	if err != nil {
		return nil, err
	}
	runtimeVersions := normalizeMap(input.RuntimeVersions)
	tools := normalizeBoolMap(input.Tools)

	issues := make([]EnvironmentIssue, 0)
	requiredRuntimes := []TemplateRuntime{spec.Backend}
	if spec.Frontend != nil {
		requiredRuntimes = append(requiredRuntimes, *spec.Frontend)
	}

	for _, rt := range requiredRuntimes {
		key := strings.ToLower(rt.Language)
		actual := runtimeVersions[key]
		if actual == "" {
			issues = append(issues, EnvironmentIssue{
				Code:     "RUNTIME_MISSING",
				Severity: "error",
				Message:  fmt.Sprintf("%s runtime is not detected", rt.Language),
				Hint:     fmt.Sprintf("Install %s %s or newer before running doctor", rt.Language, rt.MinVersion),
				Target:   key,
			})
			continue
		}
		if !compareVersions(actual, rt.MinVersion) {
			issues = append(issues, EnvironmentIssue{
				Code:     "RUNTIME_OUTDATED",
				Severity: "error",
				Message:  fmt.Sprintf("%s runtime %s is below required %s", rt.Language, safeVersion(actual), rt.MinVersion),
				Hint:     fmt.Sprintf("Upgrade %s to at least %s.", rt.Language, rt.MinVersion),
				Target:   key,
			})
		}
	}

	for _, tool := range spec.Tooling.Required {
		key := strings.ToLower(tool)
		if !tools[key] {
			issues = append(issues, EnvironmentIssue{
				Code:     "TOOL_MISSING",
				Severity: "error",
				Message:  fmt.Sprintf("Required tool %s not found in PATH", tool),
				Hint:     fmt.Sprintf("Install %s and ensure it is available in PATH.", tool),
				Target:   key,
			})
		}
	}

	report := &EnvironmentCheckReport{
		Template: spec.CloneSummary(),
		Passed:   !hasError(issues),
		Issues:   issues,
	}

	status := "passed"
	if !report.Passed {
		status = "failed"
	}
	s.emitAudit(ctx, "PLUGIN_BOOTSTRAP_DOCTOR", status, spec.ID, map[string]any{
		"issues": issues,
	})

	return report, nil
}

func (s *Service) resolveTemplate(id string) (*TemplateSpec, error) {
	targetID := id
	if targetID == "" {
		targetID = strings.TrimSpace(s.opts.DefaultTemplate)
	}
	if targetID == "" {
		return nil, errors.New("templateId is required")
	}
	spec, ok := s.registry.find(targetID)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errTemplateNotFound, targetID)
	}
	return spec, nil
}

func (s *Service) emitAudit(ctx context.Context, operation, outcome, resourceID string, meta map[string]any) {
	if s.opts.AuditSvc == nil {
		return
	}
	event := &dbm.AuditEvent{
		OccurredAt:   s.now().UTC(),
		TenantUUID:   strings.TrimSpace(reqctx.GetTenantUUID(ctx)),
		Source:       "plugin_bootstrap",
		Operation:    operation,
		ResourceType: "plugin_bootstrap",
		ResourceID:   resourceID,
		Outcome:      strings.ToUpper(outcome),
		Severity:     severityFromOutcome(outcome),
		Meta:         datatypes.JSON(marshalJSON(meta)),
	}
	if err := s.opts.AuditSvc.Emit(ctx, event); err != nil {
		pxlog.WarnF(ctx, "[plugin_bootstrap] emit audit failed: %v", err)
	}
}

func severityFromOutcome(outcome string) string {
	switch strings.ToLower(outcome) {
	case "ready", "passed":
		return "INFO"
	case "warning":
		return "WARN"
	default:
		return "ERROR"
	}
}

func defaultModulePath(pluginID string) string {
	slug := strings.NewReplacer(".", "-", "_", "-").Replace(pluginID)
	return fmt.Sprintf("github.com/powerx-plugins/%s", slug)
}

func compareVersions(actual, required string) bool {
	required = strings.TrimSpace(required)
	if required == "" {
		return true
	}
	actualNorm := normalizeVersion(actual)
	requiredNorm := normalizeVersion(required)
	if actualNorm == "" {
		return false
	}
	if requiredNorm == "" {
		return true
	}
	return semver.Compare(actualNorm, requiredNorm) >= 0
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	lower := strings.ToLower(v)
	for _, prefix := range []string{"go", "node", "v"} {
		if strings.HasPrefix(lower, prefix) && len(v) > len(prefix) {
			v = v[len(prefix):]
			break
		}
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.ContainsRune(v, ' ') {
		v = strings.SplitN(v, " ", 2)[0]
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	parts := strings.SplitN(v[1:], "-", 2)
	core := parts[0]
	if strings.Count(core, ".") == 1 {
		core += ".0"
	}
	if len(parts) > 1 {
		v = "v" + core + "-" + parts[1]
	} else {
		v = "v" + core
	}
	if semver.IsValid(v) {
		return v
	}
	return ""
}

func safeVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

func containsString(list []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, item := range list {
		if strings.ToLower(strings.TrimSpace(item)) == target {
			return true
		}
	}
	return false
}

func normalizeMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out
}

func normalizeBoolMap(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[strings.ToLower(strings.TrimSpace(k))] = v
	}
	return out
}

func hasError(issues []EnvironmentIssue) bool {
	for _, issue := range issues {
		if strings.EqualFold(issue.Severity, "error") {
			return true
		}
	}
	return false
}
