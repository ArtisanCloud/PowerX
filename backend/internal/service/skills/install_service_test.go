package skills

import "testing"

func TestNormalizeInstallRequest_MultiProvider(t *testing.T) {
	t.Run("github repo shorthand", func(t *testing.T) {
		out, err := normalizeInstallRequest(InstallTaskRequest{
			Repo: "openai/skills",
			Path: "markdown",
		})
		if err != nil {
			t.Fatalf("normalize failed: %v", err)
		}
		if out.Provider != "github" {
			t.Fatalf("expected provider github, got %s", out.Provider)
		}
		if out.RepoURL != "https://github.com/openai/skills.git" {
			t.Fatalf("unexpected repo_url: %s", out.RepoURL)
		}
	})

	t.Run("gitlab repo_url", func(t *testing.T) {
		out, err := normalizeInstallRequest(InstallTaskRequest{
			Provider: "gitlab",
			RepoURL:  "https://gitlab.com/example/repo.git",
			Path:     "skills/demo",
		})
		if err != nil {
			t.Fatalf("normalize failed: %v", err)
		}
		if out.Repo != "example/repo" {
			t.Fatalf("unexpected repo: %s", out.Repo)
		}
	})

	t.Run("generic git requires repo_url", func(t *testing.T) {
		_, err := normalizeInstallRequest(InstallTaskRequest{
			Provider: "generic_git",
			Repo:     "openai/skills",
			Path:     "markdown",
		})
		if err == nil {
			t.Fatalf("expected error for missing repo_url")
		}
	})
}
