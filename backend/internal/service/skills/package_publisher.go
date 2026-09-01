package skills

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

const canonicalSkillPackageContentType = "application/gzip"

// SkillPackageObjectStore is the sole persistence port for durable package
// bytes. PostgreSQL deliberately stores only the returned URI and checksum.
type SkillPackageObjectStore interface {
	PutSkillPackage(ctx context.Context, objectKey, contentType string, body []byte) (uri string, err error)
}

type CanonicalSkillPackageInput struct {
	TenantUUID        string
	SkillID           string
	RevisionUUID      string
	DisplayName       string
	Description       string
	Definition        map[string]any
	SourceMessageUUID string
}

type SourceSkillPackageInput struct {
	TenantUUID        string
	SkillID           string
	SourceUUID        string
	DisplayName       string
	Description       string
	Definition        map[string]any
	SourceMessageUUID string
}

type PublishedSkillPackage struct {
	ArtifactURI string
	Checksum    string
	ContentType string
	ObjectKey   string
}

// PackagePublisher writes an immutable, canonical Skill package. It has no
// database dependency, allowing the same publication path for agent-authored
// definitions and imported packages.
type PackagePublisher struct {
	store SkillPackageObjectStore
}

func NewPackagePublisher(store SkillPackageObjectStore) *PackagePublisher {
	if store == nil {
		panic("skill package publisher requires object store")
	}
	return &PackagePublisher{store: store}
}

func (p *PackagePublisher) PublishCanonical(ctx context.Context, in CanonicalSkillPackageInput) (*PublishedSkillPackage, error) {
	if p == nil || p.store == nil {
		return nil, errors.New("skill.package_publisher_unavailable")
	}
	if err := requireUUID(in.TenantUUID, "tenant_uuid"); err != nil {
		return nil, err
	}
	if err := requireUUID(in.RevisionUUID, "revision_uuid"); err != nil {
		return nil, err
	}
	if err := optionalUUID(in.SourceMessageUUID, "source_message_uuid"); err != nil {
		return nil, err
	}
	if !skillDefinitionIDPattern.MatchString(strings.ToLower(strings.TrimSpace(in.SkillID))) {
		return nil, errors.New("skill.definition_skill_id_invalid")
	}
	if strings.TrimSpace(in.DisplayName) == "" || strings.TrimSpace(in.Description) == "" {
		return nil, errors.New("skill.package_standard_manifest_invalid")
	}
	if err := validatePowerXDefinition(in.Definition); err != nil {
		return nil, err
	}
	body, err := buildSkillPackage(skillPackageBuildInput{
		TenantUUID: in.TenantUUID, SkillID: in.SkillID, DisplayName: in.DisplayName, Description: in.Description,
		Definition: in.Definition, SourceMessageUUID: in.SourceMessageUUID, ArtifactKind: "canonical_export", RevisionUUID: in.RevisionUUID,
	})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(body)
	objectKey := path.Join("skill-packages", strings.ToLower(strings.TrimSpace(in.TenantUUID)), strings.ToLower(strings.TrimSpace(in.SkillID)), strings.ToLower(strings.TrimSpace(in.RevisionUUID))+".tar.gz")
	uri, err := p.store.PutSkillPackage(ctx, objectKey, canonicalSkillPackageContentType, body)
	if err != nil {
		return nil, fmt.Errorf("skill.package_publish_failed: %w", err)
	}
	if err := requireObjectURI(uri); err != nil {
		return nil, err
	}
	return &PublishedSkillPackage{
		ArtifactURI: uri,
		Checksum:    "sha256:" + hex.EncodeToString(digest[:]),
		ContentType: canonicalSkillPackageContentType,
		ObjectKey:   objectKey,
	}, nil
}

// PublishAuthoringSource saves the structured agent draft that led to a
// definition. It is separate from the immutable canonical export generated
// during publish, even when both contain equivalent initial content.
func (p *PackagePublisher) PublishAuthoringSource(ctx context.Context, in SourceSkillPackageInput) (*PublishedSkillPackage, error) {
	if p == nil || p.store == nil {
		return nil, errors.New("skill.package_publisher_unavailable")
	}
	if err := requireUUID(in.TenantUUID, "tenant_uuid"); err != nil {
		return nil, err
	}
	if err := requireUUID(in.SourceUUID, "source_uuid"); err != nil {
		return nil, err
	}
	if err := optionalUUID(in.SourceMessageUUID, "source_message_uuid"); err != nil {
		return nil, err
	}
	if !skillDefinitionIDPattern.MatchString(strings.ToLower(strings.TrimSpace(in.SkillID))) {
		return nil, errors.New("skill.definition_skill_id_invalid")
	}
	if strings.TrimSpace(in.DisplayName) == "" || strings.TrimSpace(in.Description) == "" {
		return nil, errors.New("skill.package_standard_manifest_invalid")
	}
	if err := validatePowerXDefinition(in.Definition); err != nil {
		return nil, err
	}
	body, err := buildSkillPackage(skillPackageBuildInput{
		TenantUUID: in.TenantUUID, SkillID: in.SkillID, DisplayName: in.DisplayName, Description: in.Description,
		Definition: in.Definition, SourceMessageUUID: in.SourceMessageUUID, ArtifactKind: "agent_authoring_source", SourceUUID: in.SourceUUID,
	})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(body)
	objectKey := path.Join("skill-sources", strings.ToLower(strings.TrimSpace(in.TenantUUID)), strings.ToLower(strings.TrimSpace(in.SkillID)), strings.ToLower(strings.TrimSpace(in.SourceUUID))+".tar.gz")
	uri, err := p.store.PutSkillPackage(ctx, objectKey, canonicalSkillPackageContentType, body)
	if err != nil {
		return nil, fmt.Errorf("skill.package_publish_failed: %w", err)
	}
	if err := requireObjectURI(uri); err != nil {
		return nil, err
	}
	return &PublishedSkillPackage{ArtifactURI: uri, Checksum: "sha256:" + hex.EncodeToString(digest[:]), ContentType: canonicalSkillPackageContentType, ObjectKey: objectKey}, nil
}

type skillPackageBuildInput struct {
	TenantUUID        string
	SkillID           string
	RevisionUUID      string
	SourceUUID        string
	DisplayName       string
	Description       string
	Definition        map[string]any
	SourceMessageUUID string
	ArtifactKind      string
}

func buildSkillPackage(in skillPackageBuildInput) ([]byte, error) {
	definitionJSON, err := json.MarshalIndent(in.Definition, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("skill.package_definition_encode_failed: %w", err)
	}
	manifest := map[string]any{
		"schema":              SkillDefinitionSchemaV2,
		"skill_id":            strings.ToLower(strings.TrimSpace(in.SkillID)),
		"artifact_kind":       in.ArtifactKind,
		"source_message_uuid": strings.ToLower(strings.TrimSpace(in.SourceMessageUUID)),
	}
	if in.RevisionUUID != "" {
		manifest["revision_uuid"] = strings.ToLower(strings.TrimSpace(in.RevisionUUID))
	}
	if in.SourceUUID != "" {
		manifest["source_uuid"] = strings.ToLower(strings.TrimSpace(in.SourceUUID))
	}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("skill.package_manifest_encode_failed: %w", err)
	}
	skillMD := "---\n" +
		"name: " + strings.TrimSpace(in.DisplayName) + "\n" +
		"description: " + strings.TrimSpace(in.Description) + "\n" +
		"---\n"

	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	gz.Header.ModTime = time.Unix(0, 0).UTC()
	tw := tar.NewWriter(gz)
	for _, file := range []struct {
		name string
		body []byte
	}{
		{name: "SKILL.md", body: []byte(skillMD)},
		{name: "powerx/manifest.json", body: manifestJSON},
		{name: "powerx/definition.json", body: definitionJSON},
	} {
		if err := tw.WriteHeader(&tar.Header{Name: file.name, Mode: 0o600, Size: int64(len(file.body)), ModTime: time.Unix(0, 0).UTC()}); err != nil {
			return nil, fmt.Errorf("skill.package_archive_header_failed: %w", err)
		}
		if _, err := tw.Write(file.body); err != nil {
			return nil, fmt.Errorf("skill.package_archive_write_failed: %w", err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("skill.package_archive_close_failed: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("skill.package_compress_close_failed: %w", err)
	}
	return out.Bytes(), nil
}
