package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	wsbus "github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

const (
	defaultInstallTimeout = 180 * time.Second
	defaultSkillVersion   = "1.0.0"
)

var (
	repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
)

type InstallTaskRequest struct {
	TenantUUID string
	Provider   string
	Repo       string
	RepoURL    string
	Path       string
	Ref        string
	Method     string
	Source     string
	SkillID    string
	Version    string
	Actor      string
	AutoImport bool
}

type SkillInstallerService struct {
	repo      *skillrepo.SkillInstallTaskRepository
	importSvc *ImportService
}

func NewSkillInstallerService(repo *skillrepo.SkillInstallTaskRepository, importSvc *ImportService) *SkillInstallerService {
	if repo == nil {
		panic("skill installer service requires install task repository")
	}
	return &SkillInstallerService{
		repo:      repo,
		importSvc: importSvc,
	}
}

func (s *SkillInstallerService) CreateTask(ctx context.Context, req InstallTaskRequest) (*models.SkillInstallTask, error) {
	normalized, err := normalizeInstallRequest(req)
	if err != nil {
		return nil, err
	}
	task := &models.SkillInstallTask{
		TaskID:      uuid.NewString(),
		TenantUUID:  normalized.TenantUUID,
		Provider:    normalized.Provider,
		Repo:        normalized.Repo,
		RepoURL:     normalized.RepoURL,
		SourcePath:  normalized.Path,
		Ref:         normalized.Ref,
		Method:      normalized.Method,
		Source:      normalized.Source,
		SkillID:     normalized.SkillID,
		Version:     normalized.Version,
		Status:      models.SkillInstallStatusPending,
		RequestedBy: normalized.Actor,
	}
	created, err := s.repo.Create(ctx, task)
	if err != nil {
		return nil, err
	}
	go s.executeTask(created.TaskID, normalized.AutoImport, normalized.TenantUUID)
	return created, nil
}

func (s *SkillInstallerService) GetTask(ctx context.Context, taskID string) (*models.SkillInstallTask, error) {
	return s.repo.GetByTaskID(ctx, taskID)
}

func (s *SkillInstallerService) ListTasks(ctx context.Context, filter skillrepo.SkillInstallTaskFilter) ([]models.SkillInstallTask, int64, error) {
	return s.repo.List(ctx, filter)
}

func (s *SkillInstallerService) executeTask(taskID string, autoImport bool, fallbackTenantUUID string) {
	task, err := s.repo.GetByTaskID(context.Background(), taskID)
	if err != nil {
		return
	}
	tenantUUID := strings.TrimSpace(strings.ToLower(task.TenantUUID))
	if tenantUUID == "" {
		tenantUUID = strings.TrimSpace(strings.ToLower(fallbackTenantUUID))
	}
	startedAt := time.Now().UTC()
	_ = s.repo.UpdateFields(context.Background(), taskID, map[string]interface{}{
		"status":     models.SkillInstallStatusRunning,
		"started_at": &startedAt,
	})
	publishInstallTaskStatus(tenantUUID, task.TaskID, models.SkillInstallStatusRunning, "")

	codexHome, err := resolveCodexHome()
	if err != nil {
		s.failTask(task, tenantUUID, "", "", err)
		return
	}

	runCtx, cancel := context.WithTimeout(context.Background(), defaultInstallTimeout)
	defer cancel()

	installPath, stdoutText, stderrText, runErr := s.runInstaller(runCtx, codexHome, task)
	if runErr != nil {
		s.failTask(task, tenantUUID, stdoutText, stderrText, runErr)
		return
	}

	update := map[string]interface{}{
		"stdout_log":   stdoutText,
		"stderr_log":   stderrText,
		"install_path": installPath,
	}
	if autoImport {
		if err := s.importInstalledSkill(context.Background(), task, installPath); err != nil {
			s.failTask(task, tenantUUID, stdoutText, stderrText, err)
			return
		}
	}

	finishedAt := time.Now().UTC()
	update["status"] = models.SkillInstallStatusSuccess
	update["finished_at"] = &finishedAt
	update["error_summary"] = ""
	_ = s.repo.UpdateFields(context.Background(), taskID, update)
	publishInstallTaskStatus(tenantUUID, task.TaskID, models.SkillInstallStatusSuccess, "")
}

func (s *SkillInstallerService) runInstaller(ctx context.Context, codexHome string, task *models.SkillInstallTask) (string, string, string, error) {
	if task == nil {
		return "", "", "", errors.New("install task is nil")
	}
	provider := strings.TrimSpace(strings.ToLower(task.Provider))
	if provider == "" {
		provider = "github"
	}
	if provider == "github" {
		installPath, stdoutText, stderrText, err := runGitHubScriptInstaller(ctx, codexHome, task)
		if err == nil {
			return installPath, stdoutText, stderrText, nil
		}
		// GitHub script is best-effort; fallback to generic git installer.
		return runGenericGitInstaller(ctx, codexHome, task, stdoutText, stderrText)
	}
	return runGenericGitInstaller(ctx, codexHome, task, "", "")
}

func runGitHubScriptInstaller(ctx context.Context, codexHome string, task *models.SkillInstallTask) (string, string, string, error) {
	scriptPath := resolveInstallScriptPath(codexHome)
	if _, err := os.Stat(scriptPath); err != nil {
		return "", "", "", fmt.Errorf("installer script not found: %w", err)
	}
	args := []string{
		scriptPath,
		"--repo", strings.TrimSpace(task.Repo),
		"--path", strings.TrimSpace(task.SourcePath),
		"--ref", strings.TrimSpace(task.Ref),
		"--method", strings.TrimSpace(task.Method),
	}
	if repoURL := strings.TrimSpace(task.RepoURL); repoURL != "" {
		args = []string{
			scriptPath,
			"--url", repoURL,
			"--path", strings.TrimSpace(task.SourcePath),
			"--ref", strings.TrimSpace(task.Ref),
			"--method", strings.TrimSpace(task.Method),
		}
	}
	cmd := exec.CommandContext(ctx, "python3", args...)
	cmd.Env = append(os.Environ(), "CODEX_HOME="+codexHome)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	stdoutText := strings.TrimSpace(stdout.String())
	stderrText := strings.TrimSpace(stderr.String())
	if err != nil {
		return "", stdoutText, stderrText, fmt.Errorf("install script failed: %w; %s", err, stderrText)
	}
	installPath := filepath.Join(codexHome, "skills", filepath.Base(task.SourcePath))
	return installPath, stdoutText, stderrText, nil
}

func runGenericGitInstaller(ctx context.Context, codexHome string, task *models.SkillInstallTask, prefixStdout, prefixStderr string) (string, string, string, error) {
	repoURL := strings.TrimSpace(task.RepoURL)
	if repoURL == "" {
		return "", prefixStdout, prefixStderr, errors.New("repo_url is required for generic git installation")
	}
	if strings.EqualFold(strings.TrimSpace(task.Method), "download") {
		return "", prefixStdout, prefixStderr, errors.New("download method is only supported by github installer")
	}
	tmpDir, err := os.MkdirTemp("", "powerx-skill-install-*")
	if err != nil {
		return "", prefixStdout, prefixStderr, fmt.Errorf("create temp dir failed: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	repoDir := filepath.Join(tmpDir, "repo")
	var stdoutChunks []string
	var stderrChunks []string
	if prefixStdout != "" {
		stdoutChunks = append(stdoutChunks, "[github-script fallback]\n"+prefixStdout)
	}
	if prefixStderr != "" {
		stderrChunks = append(stderrChunks, "[github-script fallback]\n"+prefixStderr)
	}
	stdoutText, stderrText, err := runCommand(ctx, "git", []string{"clone", "--filter=blob:none", "--depth", "1", "--sparse", "--single-branch", "--branch", strings.TrimSpace(task.Ref), repoURL, repoDir}...)
	stdoutChunks = appendIfNonEmpty(stdoutChunks, stdoutText)
	stderrChunks = appendIfNonEmpty(stderrChunks, stderrText)
	if err != nil {
		stdoutText2, stderrText2, err2 := runCommand(ctx, "git", []string{"clone", "--filter=blob:none", "--depth", "1", "--sparse", "--single-branch", repoURL, repoDir}...)
		stdoutChunks = appendIfNonEmpty(stdoutChunks, stdoutText2)
		stderrChunks = appendIfNonEmpty(stderrChunks, stderrText2)
		if err2 != nil {
			return "", strings.Join(stdoutChunks, "\n"), strings.Join(stderrChunks, "\n"), fmt.Errorf("git clone failed: %w", err2)
		}
	}
	stdoutText, stderrText, err = runCommand(ctx, "git", []string{"-C", repoDir, "sparse-checkout", "set", strings.TrimSpace(task.SourcePath)}...)
	stdoutChunks = appendIfNonEmpty(stdoutChunks, stdoutText)
	stderrChunks = appendIfNonEmpty(stderrChunks, stderrText)
	if err != nil {
		return "", strings.Join(stdoutChunks, "\n"), strings.Join(stderrChunks, "\n"), fmt.Errorf("git sparse-checkout failed: %w", err)
	}
	stdoutText, stderrText, err = runCommand(ctx, "git", []string{"-C", repoDir, "checkout", strings.TrimSpace(task.Ref)}...)
	stdoutChunks = appendIfNonEmpty(stdoutChunks, stdoutText)
	stderrChunks = appendIfNonEmpty(stderrChunks, stderrText)
	if err != nil {
		return "", strings.Join(stdoutChunks, "\n"), strings.Join(stderrChunks, "\n"), fmt.Errorf("git checkout failed: %w", err)
	}

	sourceDir := filepath.Join(repoDir, filepath.FromSlash(strings.TrimSpace(task.SourcePath)))
	if fi, statErr := os.Stat(filepath.Join(sourceDir, "SKILL.md")); statErr != nil || fi.IsDir() {
		return "", strings.Join(stdoutChunks, "\n"), strings.Join(stderrChunks, "\n"), errors.New("SKILL.md not found in selected skill directory")
	}
	destDir := filepath.Join(codexHome, "skills", filepath.Base(strings.TrimSpace(task.SourcePath)))
	if _, statErr := os.Stat(destDir); statErr == nil {
		return "", strings.Join(stdoutChunks, "\n"), strings.Join(stderrChunks, "\n"), fmt.Errorf("destination already exists: %s", destDir)
	}
	if err := copyDir(sourceDir, destDir); err != nil {
		return "", strings.Join(stdoutChunks, "\n"), strings.Join(stderrChunks, "\n"), fmt.Errorf("copy installed skill failed: %w", err)
	}
	return destDir, strings.Join(stdoutChunks, "\n"), strings.Join(stderrChunks, "\n"), nil
}

func runCommand(ctx context.Context, command string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

func copyDir(srcDir, dstDir string) error {
	srcDir = filepath.Clean(srcDir)
	dstDir = filepath.Clean(dstDir)
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			info, infoErr := d.Info()
			if infoErr != nil {
				return infoErr
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		dstFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			return err
		}
		defer dstFile.Close()
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return err
		}
		return nil
	})
}

func appendIfNonEmpty(items []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return items
	}
	return append(items, value)
}

func (s *SkillInstallerService) failTask(task *models.SkillInstallTask, tenantUUID, stdoutLog, stderrLog string, err error) {
	if task == nil {
		return
	}
	finishedAt := time.Now().UTC()
	msg := strings.TrimSpace(err.Error())
	_ = s.repo.UpdateFields(context.Background(), task.TaskID, map[string]interface{}{
		"status":        models.SkillInstallStatusFailed,
		"finished_at":   &finishedAt,
		"stdout_log":    strings.TrimSpace(stdoutLog),
		"stderr_log":    strings.TrimSpace(stderrLog),
		"error_summary": msg,
	})
	publishInstallTaskStatus(tenantUUID, task.TaskID, models.SkillInstallStatusFailed, msg)
}

func (s *SkillInstallerService) importInstalledSkill(ctx context.Context, task *models.SkillInstallTask, installPath string) error {
	if s == nil || s.importSvc == nil || task == nil {
		return nil
	}
	skillMarkdown, err := os.ReadFile(filepath.Join(installPath, "SKILL.md"))
	if err != nil {
		return fmt.Errorf("read installed SKILL.md failed: %w", err)
	}
	manifest, err := ParseSkillMarkdownToManifest(string(skillMarkdown), task.Version)
	if err != nil {
		return fmt.Errorf("parse installed SKILL.md failed: %w", err)
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal installed manifest failed: %w", err)
	}
	skillID := strings.TrimSpace(task.SkillID)
	if skillID == "" {
		skillID = strings.ToLower(filepath.Base(task.SourcePath))
	}
	record, err := s.importSvc.ImportDraft(ctx, ImportRequest{
		SkillID:    skillID,
		Version:    strings.TrimSpace(task.Version),
		Source:     strings.TrimSpace(task.Source),
		BundleURI:  "file://" + installPath,
		SourceURL:  installSourceURL(task),
		SourceRef:  strings.TrimSpace(task.Ref),
		SourcePath: strings.TrimSpace(task.SourcePath),
		Manifest:   datatypes.JSON(raw),
		Operator:   "skills.installer",
		ImportType: ImportTypeMarketplace,
	})
	if err != nil {
		return fmt.Errorf("import installed skill failed: %w", err)
	}
	return s.repo.UpdateFields(ctx, task.TaskID, map[string]interface{}{
		"skill_id": record.SkillID,
		"version":  record.Version,
	})
}

func normalizeInstallRequest(req InstallTaskRequest) (InstallTaskRequest, error) {
	out := req
	out.TenantUUID = strings.TrimSpace(strings.ToLower(out.TenantUUID))
	out.Provider = strings.ToLower(strings.TrimSpace(out.Provider))
	out.Repo = strings.ToLower(strings.TrimSpace(out.Repo))
	out.RepoURL = strings.TrimSpace(out.RepoURL)
	out.Path = strings.TrimSpace(out.Path)
	out.Ref = strings.TrimSpace(out.Ref)
	out.Method = strings.ToLower(strings.TrimSpace(out.Method))
	out.Source = strings.ToLower(strings.TrimSpace(out.Source))
	out.SkillID = strings.ToLower(strings.TrimSpace(out.SkillID))
	out.Version = strings.TrimSpace(out.Version)
	out.Actor = strings.TrimSpace(out.Actor)
	if out.Path == "" {
		return out, errors.New("path is required")
	}
	if filepath.IsAbs(out.Path) || strings.Contains(out.Path, "..") {
		return out, errors.New("path must be a relative path without '..'")
	}
	if out.Repo == "" && out.RepoURL == "" {
		return out, errors.New("repo or repo_url is required")
	}
	if strings.Contains(out.Repo, "://") && out.RepoURL == "" {
		out.RepoURL = out.Repo
		out.Repo = ""
	}
	if out.Provider == "" {
		out.Provider = inferInstallProvider(out.Repo, out.RepoURL)
	}
	switch out.Provider {
	case "github", "gitlab", "gitee", "bitbucket", "generic_git":
	default:
		return out, errors.New("provider must be github, gitlab, gitee, bitbucket, or generic_git")
	}
	if out.Repo != "" && !repoPattern.MatchString(out.Repo) {
		return out, errors.New("repo must be in owner/repo format")
	}
	if out.RepoURL == "" {
		switch out.Provider {
		case "github", "gitlab", "gitee", "bitbucket":
			if out.Repo == "" {
				return out, errors.New("repo is required")
			}
			out.RepoURL = buildRepoURL(out.Provider, out.Repo)
		default:
			return out, errors.New("repo_url is required for provider generic_git")
		}
	}
	host, repoFromURL := parseRepoFromURL(out.RepoURL)
	if out.Provider != "generic_git" {
		if !providerHostMatches(out.Provider, host) {
			return out, fmt.Errorf("repo_url host does not match provider %s", out.Provider)
		}
		if out.Repo == "" && repoFromURL != "" {
			out.Repo = repoFromURL
		}
	}
	if out.Repo == "" {
		if repoFromURL != "" {
			out.Repo = repoFromURL
		} else {
			out.Repo = strings.ToLower(out.RepoURL)
		}
	}
	if out.Ref == "" {
		out.Ref = "main"
	}
	switch out.Method {
	case "", "auto":
		out.Method = "auto"
	case "download", "git":
	default:
		return out, errors.New("method must be auto, download, or git")
	}
	if out.Source == "" {
		out.Source = models.SkillSourceThirdParty
	}
	switch out.Source {
	case models.SkillSourceThirdParty, models.SkillSourcePlugin:
	default:
		return out, errors.New("source must be third_party or plugin")
	}
	if out.SkillID == "" {
		out.SkillID = strings.ToLower(filepath.Base(out.Path))
	}
	if out.Version == "" {
		out.Version = defaultSkillVersion
	}
	if out.Actor == "" {
		out.Actor = "system"
	}
	return out, nil
}

func publishInstallTaskStatus(tenantUUID, taskID, status, errorSummary string) {
	tenantUUID = strings.TrimSpace(strings.ToLower(tenantUUID))
	taskID = strings.TrimSpace(taskID)
	status = strings.TrimSpace(strings.ToLower(status))
	if tenantUUID == "" || taskID == "" || status == "" {
		return
	}

	title := "Skill 安装任务更新"
	content := "任务 " + taskID + " 状态：" + status
	level := "info"
	if status == models.SkillInstallStatusSuccess {
		level = "success"
		content = "任务 " + taskID + " 安装并导入成功"
	}
	if status == models.SkillInstallStatusFailed {
		level = "error"
		if strings.TrimSpace(errorSummary) != "" {
			content = strings.TrimSpace(errorSummary)
		}
	}
	payload := map[string]any{
		"type":          level,
		"title":         title,
		"content":       content,
		"kind":          "skills.install.task",
		"task_id":       taskID,
		"status":        status,
		"error_summary": strings.TrimSpace(errorSummary),
		"createdAt":     time.Now().UTC().Format(time.RFC3339),
		"isRead":        false,
	}
	wsbus.DefaultHub.Publish(tenantUUID, eventbus.TopicSystemNotification, payload, "")
}

func installSourceURL(task *models.SkillInstallTask) string {
	if task == nil {
		return ""
	}
	repoURL := strings.TrimSpace(task.RepoURL)
	if repoURL != "" {
		return repoURL
	}
	provider := strings.TrimSpace(strings.ToLower(task.Provider))
	repo := strings.TrimSpace(task.Repo)
	if provider == "" {
		provider = "github"
	}
	if repo == "" {
		return ""
	}
	if repoPattern.MatchString(repo) {
		return buildRepoURL(provider, repo)
	}
	return repo
}

func inferInstallProvider(repo, repoURL string) string {
	if repoURL != "" {
		host, _ := parseRepoFromURL(repoURL)
		switch {
		case strings.Contains(host, "github.com"):
			return "github"
		case strings.Contains(host, "gitlab.com"):
			return "gitlab"
		case strings.Contains(host, "gitee.com"):
			return "gitee"
		case strings.Contains(host, "bitbucket.org"):
			return "bitbucket"
		default:
			return "generic_git"
		}
	}
	if repoPattern.MatchString(strings.TrimSpace(repo)) {
		return "github"
	}
	return "generic_git"
}

func buildRepoURL(provider, repo string) string {
	repo = strings.TrimSpace(strings.TrimSuffix(repo, ".git"))
	switch provider {
	case "gitlab":
		return "https://gitlab.com/" + repo + ".git"
	case "gitee":
		return "https://gitee.com/" + repo + ".git"
	case "bitbucket":
		return "https://bitbucket.org/" + repo + ".git"
	case "github":
		fallthrough
	default:
		return "https://github.com/" + repo + ".git"
	}
}

func parseRepoFromURL(repoURL string) (host string, repo string) {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return "", ""
	}
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return "", ""
	}
	host = strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	path := strings.Trim(strings.TrimSpace(parsed.Path), "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return host, ""
	}
	repo = strings.ToLower(parts[0] + "/" + strings.TrimSuffix(parts[1], ".git"))
	if repoPattern.MatchString(repo) {
		return host, repo
	}
	return host, ""
}

func providerHostMatches(provider, host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	switch provider {
	case "github":
		return strings.Contains(host, "github.com")
	case "gitlab":
		return strings.Contains(host, "gitlab.com")
	case "gitee":
		return strings.Contains(host, "gitee.com")
	case "bitbucket":
		return strings.Contains(host, "bitbucket.org")
	case "generic_git":
		return true
	default:
		return false
	}
}

func resolveCodexHome() (string, error) {
	if env := strings.TrimSpace(os.Getenv("CODEX_HOME")); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

func resolveInstallScriptPath(codexHome string) string {
	if custom := strings.TrimSpace(os.Getenv("SKILL_INSTALLER_SCRIPT")); custom != "" {
		return custom
	}
	return filepath.Join(codexHome, "skills", ".system", "skill-installer", "scripts", "install-skill-from-github.py")
}

func IsInstallTaskNotFound(err error) bool {
	return errors.Is(err, skillrepo.ErrSkillInstallTaskNotFound) || errors.Is(err, gorm.ErrRecordNotFound)
}
