package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type SkillSourceResolved struct {
	SkillMarkdown string
	BundleURI     string
	SourceRef     string
}

type SkillSourceResolver interface {
	Resolve(ctx context.Context, req ImportRequest) (*SkillSourceResolved, error)
}

type SkillSourcePreviewer interface {
	Preview(ctx context.Context, req ImportRequest) (*MarketplacePreview, error)
}

type MarketplacePreview struct {
	Provider         string   `json:"provider"`
	Owner            string   `json:"owner"`
	Repo             string   `json:"repo"`
	Ref              string   `json:"ref"`
	ResolvedPath     string   `json:"resolved_path,omitempty"`
	ResolvedRawURL   string   `json:"resolved_raw_url,omitempty"`
	CheckedPaths     []string `json:"checked_paths,omitempty"`
	SuggestedPaths   []string `json:"suggested_paths,omitempty"`
	InstallablePaths []string `json:"installable_paths,omitempty"`
}

type githubSkillSourceResolver struct {
	httpClient *http.Client
	repoBase   string
	rawBase    string
	apiBase    string
}

func NewGitHubSkillSourceResolver() SkillSourceResolver {
	return &githubSkillSourceResolver{
		httpClient: &http.Client{Timeout: 12 * time.Second},
		repoBase:   "https://github.com",
		rawBase:    "https://raw.githubusercontent.com",
		apiBase:    "https://api.github.com",
	}
}

func (r *githubSkillSourceResolver) Resolve(ctx context.Context, req ImportRequest) (*SkillSourceResolved, error) {
	if r == nil || r.httpClient == nil {
		return nil, fmt.Errorf("github source resolver is not initialized")
	}
	sourceURL := strings.TrimSpace(req.SourceURL)
	if sourceURL == "" {
		return nil, fmt.Errorf("source_url is required")
	}
	fallbackRef := strings.TrimSpace(req.SourceRef)
	if fallbackRef == "" {
		fallbackRef = "main"
	}
	target, err := parseGitHubSource(sourceURL, fallbackRef, req.SourcePath)
	if err != nil {
		return nil, err
	}

	skillSlug := skillSlugFromID(req.SkillID)
	candidates := buildSkillMarkdownCandidates(target.Path, skillSlug)

	var markdown string
	var foundPath string
	for _, rel := range candidates {
		rawURL := strings.TrimRight(r.rawBase, "/") + "/" + target.Owner + "/" + target.Repo + "/" + target.Ref + "/" + rel
		content, fetchErr := r.fetchText(ctx, rawURL)
		if fetchErr == nil && strings.TrimSpace(content) != "" {
			markdown = content
			foundPath = rel
			break
		}
	}
	if strings.TrimSpace(markdown) == "" {
		return nil, fmt.Errorf("cannot find SKILL.md from source_url=%s ref=%s", sourceURL, target.Ref)
	}

	bundleURI := strings.TrimRight(r.repoBase, "/") + "/" + target.Owner + "/" + target.Repo + "/tree/" + target.Ref
	if foundPath != "" && foundPath != "SKILL.md" {
		bundleURI = bundleURI + "/" + strings.TrimSuffix(foundPath, "/SKILL.md")
	}
	return &SkillSourceResolved{
		SkillMarkdown: markdown,
		BundleURI:     bundleURI,
		SourceRef:     target.Ref,
	}, nil
}

func (r *githubSkillSourceResolver) Preview(ctx context.Context, req ImportRequest) (*MarketplacePreview, error) {
	if r == nil || r.httpClient == nil {
		return nil, fmt.Errorf("github source resolver is not initialized")
	}
	sourceURL := strings.TrimSpace(req.SourceURL)
	if sourceURL == "" {
		return nil, fmt.Errorf("source_url is required")
	}
	fallbackRef := strings.TrimSpace(req.SourceRef)
	if fallbackRef == "" {
		fallbackRef = "main"
	}
	target, err := parseGitHubSource(sourceURL, fallbackRef, req.SourcePath)
	if err != nil {
		return nil, err
	}

	skillSlug := skillSlugFromID(req.SkillID)
	candidates := buildSkillMarkdownCandidates(target.Path, skillSlug)
	checked := make([]string, 0, len(candidates))
	resolvedPath := ""
	resolvedRawURL := ""
	for _, rel := range candidates {
		rawURL := strings.TrimRight(r.rawBase, "/") + "/" + target.Owner + "/" + target.Repo + "/" + target.Ref + "/" + rel
		checked = append(checked, rel)
		if r.headOK(ctx, rawURL) {
			resolvedPath = rel
			resolvedRawURL = rawURL
			break
		}
	}

	suggested := []string{}
	if skillSlug != "" {
		suggested = append(suggested, "skills/"+skillSlug, skillSlug)
	}
	return &MarketplacePreview{
		Provider:         "github",
		Owner:            target.Owner,
		Repo:             target.Repo,
		Ref:              target.Ref,
		ResolvedPath:     resolvedPath,
		ResolvedRawURL:   resolvedRawURL,
		CheckedPaths:     checked,
		SuggestedPaths:   uniqueStrings(suggested),
		InstallablePaths: r.listInstallableSkillPaths(ctx, target.Owner, target.Repo, target.Ref),
	}, nil
}

func (r *githubSkillSourceResolver) fetchText(ctx context.Context, target string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (r *githubSkillSourceResolver) headOK(ctx context.Context, target string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return false
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (r *githubSkillSourceResolver) listInstallableSkillPaths(ctx context.Context, owner, repo, ref string) []string {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "" || strings.TrimSpace(ref) == "" {
		return nil
	}
	apiURL := strings.TrimRight(r.apiBase, "/") + "/repos/" + owner + "/" + repo + "/contents/skills?ref=" + url.QueryEscape(ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil
	}
	type entry struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	var entries []entry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, item := range entries {
		if strings.EqualFold(strings.TrimSpace(item.Type), "dir") && strings.TrimSpace(item.Name) != "" {
			out = append(out, "skills/"+strings.TrimSpace(item.Name))
		}
	}
	return uniqueStrings(out)
}

type githubSourceTarget struct {
	Owner string
	Repo  string
	Ref   string
	Path  string
}

func parseGitHubSource(raw, fallbackRef, fallbackPath string) (*githubSourceTarget, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid source_url: %w", err)
	}
	if !strings.EqualFold(u.Host, "github.com") {
		return nil, fmt.Errorf("only github.com source_url is supported for marketplace import")
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 2 {
		return nil, fmt.Errorf("source_url must be https://github.com/<owner>/<repo>")
	}
	target := &githubSourceTarget{
		Owner: strings.TrimSpace(segments[0]),
		Repo:  strings.TrimSpace(segments[1]),
		Ref:   strings.TrimSpace(fallbackRef),
		Path:  strings.Trim(strings.TrimSpace(fallbackPath), "/"),
	}
	if target.Ref == "" {
		target.Ref = "main"
	}
	if len(segments) >= 4 && (segments[2] == "tree" || segments[2] == "blob") {
		if strings.TrimSpace(segments[3]) != "" {
			target.Ref = strings.TrimSpace(segments[3])
		}
		if len(segments) > 4 && target.Path == "" {
			target.Path = strings.Trim(strings.Join(segments[4:], "/"), "/")
		}
	}
	return target, nil
}

func skillSlugFromID(skillID string) string {
	id := strings.TrimSpace(strings.ToLower(skillID))
	if id == "" {
		return "skill"
	}
	parts := strings.Split(id, ".")
	return strings.TrimSpace(parts[len(parts)-1])
}

func buildSkillMarkdownCandidates(targetPath, skillSlug string) []string {
	candidates := make([]string, 0, 8)
	if p := strings.Trim(targetPath, "/"); p != "" {
		if strings.HasSuffix(strings.ToUpper(p), "/SKILL.MD") || strings.EqualFold(path.Base(p), "SKILL.md") {
			candidates = append(candidates, p)
		} else {
			candidates = append(candidates, path.Join(p, "SKILL.md"))
		}
	}
	candidates = append(candidates, "SKILL.md")
	if skillSlug != "" {
		candidates = append(candidates, path.Join("skills", skillSlug, "SKILL.md"))
		candidates = append(candidates, path.Join(skillSlug, "SKILL.md"))
	}
	return uniqueStrings(candidates)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		k := strings.TrimSpace(v)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}
