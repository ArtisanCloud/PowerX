package skills

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGitHubSource_TreeURL(t *testing.T) {
	target, err := parseGitHubSource(
		"https://github.com/anthropics/skills/tree/main/skills/skill-creator",
		"main",
		"",
	)
	require.NoError(t, err)
	require.Equal(t, "anthropics", target.Owner)
	require.Equal(t, "skills", target.Repo)
	require.Equal(t, "main", target.Ref)
	require.Equal(t, "skills/skill-creator", target.Path)
}

func TestParseGitHubSource_ExplicitPath(t *testing.T) {
	target, err := parseGitHubSource(
		"https://github.com/anthropics/skills",
		"main",
		"skills/skill-creator",
	)
	require.NoError(t, err)
	require.Equal(t, "main", target.Ref)
	require.Equal(t, "skills/skill-creator", target.Path)
}
