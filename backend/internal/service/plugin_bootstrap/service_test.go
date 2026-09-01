package plugin_bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	auditpkg "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/stretchr/testify/require"
)

func TestValidateBootstrapUsesDefaultTemplate(t *testing.T) {
	ctx := context.Background()
	path := writeTempFile(t, `
templates:
  - id: fullstack-go-nuxt
    name: Default
    description: test
    cli:
      min_version: v0.2.0
    backend:
      language: go
      framework: gin
      template: go-gin
      min_version: 1.26.7
    tooling:
      required: ["git","go"]
`)
	svc, err := NewService(Options{
		TemplatesPath:   path,
		DefaultTemplate: "fullstack-go-nuxt",
		Auditor:         auditpkg.Noop{},
		AuditSvc:        &noopAudit{},
	})
	require.NoError(t, err)

	result, err := svc.ValidateBootstrap(ctx, BootstrapValidateInput{
		PluginID:   "com.powerx.demo",
		CLIVersion: "v0.3.0",
	})
	require.NoError(t, err)
	require.Equal(t, "ready", result.Status)
	require.Equal(t, "gin", result.Template.Backend.Framework)
	require.Len(t, result.Issues, 0)
}

func TestCheckEnvironmentDetectsIssues(t *testing.T) {
	ctx := context.Background()
	path := writeTempFile(t, `
templates:
  - id: backend-go-lite
    name: Backend Lite
    description: headless
    cli:
      min_version: v0.2.0
    backend:
      language: go
      framework: gin
      template: go-gin
      min_version: 1.26.7
    tooling:
      required: ["git","docker"]
`)
	svc, err := NewService(Options{
		TemplatesPath:   path,
		DefaultTemplate: "backend-go-lite",
		Auditor:         auditpkg.Noop{},
		AuditSvc:        &noopAudit{},
	})
	require.NoError(t, err)

	report, err := svc.CheckEnvironment(ctx, EnvironmentCheckInput{
		RuntimeVersions: map[string]string{"go": "1.20.0"},
		Tools:           map[string]bool{"git": true, "docker": false},
	})
	require.NoError(t, err)
	require.False(t, report.Passed)
	require.Len(t, report.Issues, 2)
}

type noopAudit struct{}

func (noopAudit) Emit(context.Context, *dbm.AuditEvent) error { return nil }
func (noopAudit) Close()                                    {}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "index.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}
