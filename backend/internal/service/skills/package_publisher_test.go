package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type memorySkillPackageStore struct {
	key         string
	contentType string
	body        []byte
}

func (s *memorySkillPackageStore) PutSkillPackage(_ context.Context, key, contentType string, body []byte) (string, error) {
	s.key = key
	s.contentType = contentType
	s.body = append([]byte(nil), body...)
	return "s3://powerx-media/" + key, nil
}

func TestPackagePublisherWritesCanonicalPortablePackage(t *testing.T) {
	store := &memorySkillPackageStore{}
	publisher := NewPackagePublisher(store)
	tenantUUID := uuid.NewString()
	revisionUUID := uuid.NewString()
	result, err := publisher.PublishCanonical(context.Background(), CanonicalSkillPackageInput{
		TenantUUID: tenantUUID, SkillID: "tenant.custom_review", RevisionUUID: revisionUUID,
		DisplayName: "自定义复盘", Description: "按输入生成复盘报告",
		Definition: map[string]any{
			"schema":   SkillDefinitionSchemaV2,
			"executor": map[string]any{"type": "llm_prompt", "prompt_template_i18n": map[string]any{"zh-CN": "输出 Markdown"}},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "application/gzip", result.ContentType)
	require.Contains(t, result.ArtifactURI, "s3://powerx-media/skill-packages/")
	require.NotEmpty(t, result.Checksum)
	require.NotEmpty(t, store.body)

	gz, err := gzip.NewReader(bytes.NewReader(store.body))
	require.NoError(t, err)
	defer gz.Close()
	tarReader := tar.NewReader(gz)
	files := map[string]string{}
	for {
		header, readErr := tarReader.Next()
		if readErr == io.EOF {
			break
		}
		require.NoError(t, readErr)
		content, readErr := io.ReadAll(tarReader)
		require.NoError(t, readErr)
		files[header.Name] = string(content)
	}
	require.Contains(t, files, "SKILL.md")
	require.Contains(t, files, "powerx/manifest.json")
	require.Contains(t, files, "powerx/definition.json")
	require.Contains(t, files["powerx/definition.json"], "llm_prompt")
}
