package knowledge_space

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/google/uuid"
)

type ArtifactStoreOptions struct {
	Scheme  string
	Bucket  string
	BaseDir string
}

type ArtifactStore struct {
	scheme  string
	bucket  string
	baseDir string
}

func NewArtifactStore(opts ArtifactStoreOptions) *ArtifactStore {
	scheme := strings.TrimSpace(opts.Scheme)
	if scheme == "" {
		scheme = "minio"
	}
	bucket := strings.TrimSpace(opts.Bucket)
	if bucket == "" {
		bucket = "powerx-knowledge"
	}
	baseDir := strings.TrimSpace(opts.BaseDir)
	if baseDir == "" {
		if isTestBinary() {
			baseDir = filepath.Join(projectTmpDir(), "knowledge-artifacts")
		} else {
			baseDir = filepath.Join("backend", "reports", "_state", "knowledge-artifacts")
		}
	}
	return &ArtifactStore{scheme: scheme, bucket: bucket, baseDir: baseDir}
}

type ArtifactWriteInput struct {
	SpaceID        uuid.UUID
	JobUUID        uuid.UUID
	JobID          uint64
	Format         string
	SourceURI      string
	Chunks         []IngestionChunk
	VectorRecords  []vectorstore.VectorRecord
	MaskingProfile string
	Outcome        pipelineOutcome
}

type ArtifactBundleUpdate struct {
	ChunkManifestURI  string
	VectorManifestURI string
	MaskingReportURI  string
	Checksum          string
}

func (s *ArtifactStore) Write(ctx context.Context, in ArtifactWriteInput) (ArtifactBundleUpdate, error) {
	_ = ctx
	if s == nil {
		return ArtifactBundleUpdate{}, nil
	}
	objectPrefix := filepath.ToSlash(filepath.Join("knowledge", in.SpaceID.String(), in.JobUUID.String()))
	chunkKey := objectPrefix + "/chunk_manifest.json"
	vectorKey := objectPrefix + "/vector_manifest.json"
	maskingKey := objectPrefix + "/masking_report.json"

	chunkManifest := map[string]any{
		"space_id":   in.SpaceID.String(),
		"job_id":     in.JobUUID.String(),
		"format":     in.Format,
		"source_uri": in.SourceURI,
		"chunks":     in.Chunks,
		"metrics":    in.Outcome,
	}
	vectorManifest := map[string]any{
		"space_id":   in.SpaceID.String(),
		"job_id":     in.JobUUID.String(),
		"dimensions": 32,
		"model":      "hash32",
		"vectors":    in.VectorRecords,
	}
	maskingReport := map[string]any{
		"space_id":        in.SpaceID.String(),
		"job_id":          in.JobUUID.String(),
		"masking_profile": in.MaskingProfile,
		"masking_pct":     in.Outcome.maskingPct,
	}

	chunkBytes, _ := json.MarshalIndent(chunkManifest, "", "  ")
	vectorBytes, _ := json.MarshalIndent(vectorManifest, "", "  ")
	maskingBytes, _ := json.MarshalIndent(maskingReport, "", "  ")

	check := sha256.New()
	check.Write(chunkBytes)
	check.Write(vectorBytes)
	check.Write(maskingBytes)
	checksum := hex.EncodeToString(check.Sum(nil))

	if err := s.writeObject(chunkKey, chunkBytes); err != nil {
		return ArtifactBundleUpdate{}, err
	}
	if err := s.writeObject(vectorKey, vectorBytes); err != nil {
		return ArtifactBundleUpdate{}, err
	}
	if err := s.writeObject(maskingKey, maskingBytes); err != nil {
		return ArtifactBundleUpdate{}, err
	}

	return ArtifactBundleUpdate{
		ChunkManifestURI:  s.uri(chunkKey),
		VectorManifestURI: s.uri(vectorKey),
		MaskingReportURI:  s.uri(maskingKey),
		Checksum:          checksum,
	}, nil
}

func (s *ArtifactStore) writeObject(objectKey string, data []byte) error {
	if s == nil {
		return nil
	}
	path := filepath.Join(s.baseDir, s.bucket, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func (s *ArtifactStore) uri(objectKey string) string {
	if s == nil {
		return ""
	}
	return s.scheme + "://" + s.bucket + "/" + strings.TrimPrefix(objectKey, "/")
}
