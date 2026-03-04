package knowledge_space

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
			wd, err := os.Getwd()
			if err != nil {
				baseDir = filepath.Join("backend", "reports", "_state", "knowledge-artifacts")
			} else if root := findRepoRoot(wd); root != "" {
				baseDir = filepath.Join(root, "backend", "reports", "_state", "knowledge-artifacts")
			} else {
				baseDir = filepath.Join("backend", "reports", "_state", "knowledge-artifacts")
			}
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
	OCRArtifacts   *OCRArtifacts
}

type ArtifactBundleUpdate struct {
	ChunkManifestURI    string
	VectorManifestURI   string
	MaskingReportURI    string
	OCRPageImagesURI    string
	OCRRawManifestURI   string
	OCRSearchablePDFURI string
	Checksum            string
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
	pagesManifestKey := objectPrefix + "/ocr/pages_manifest.json"
	rawManifestKey := objectPrefix + "/ocr/raw_manifest.json"

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

	var pagesManifestBytes []byte
	var rawManifestBytes []byte
	var pagesManifestURI string
	var rawManifestURI string
	var searchablePDFURI string

	if in.OCRArtifacts != nil && len(in.OCRArtifacts.Pages) > 0 {
		type pageItem struct {
			PageNumber int    `json:"page_number"`
			ImageURI   string `json:"image_uri"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
		}
		type rawItem struct {
			PageNumber int    `json:"page_number"`
			Format     string `json:"format"`
			RawURI     string `json:"raw_uri"`
		}
		pageItems := make([]pageItem, 0, len(in.OCRArtifacts.Pages))
		rawItems := make([]rawItem, 0, len(in.OCRArtifacts.Pages))
		for _, pg := range in.OCRArtifacts.Pages {
			if pg.PageNumber <= 0 {
				continue
			}
			imgExt := pg.ImageExt()
			if imgExt == "" {
				imgExt = ".png"
			}
			rawExt := pg.RawExt()
			if rawExt == "" {
				rawExt = ".tsv"
			}
			imgKey := fmt.Sprintf("%s/ocr/pages/%03d%s", objectPrefix, pg.PageNumber, imgExt)
			rawKey := fmt.Sprintf("%s/ocr/raw/%03d%s", objectPrefix, pg.PageNumber, rawExt)

			if b, err := os.ReadFile(filepath.Clean(pg.ImagePath)); err == nil {
				if err := s.writeObject(imgKey, b); err != nil {
					return ArtifactBundleUpdate{}, err
				}
				pageItems = append(pageItems, pageItem{
					PageNumber: pg.PageNumber,
					ImageURI:   s.uri(imgKey),
					Width:      pg.Width,
					Height:     pg.Height,
				})
			}
			if b, err := os.ReadFile(filepath.Clean(pg.RawPath)); err == nil {
				if err := s.writeObject(rawKey, b); err != nil {
					return ArtifactBundleUpdate{}, err
				}
				rawItems = append(rawItems, rawItem{
					PageNumber: pg.PageNumber,
					Format:     strings.TrimSpace(in.OCRArtifacts.RawFormat),
					RawURI:     s.uri(rawKey),
				})
			}
		}
		pagesManifest := map[string]any{
			"space_id":   in.SpaceID.String(),
			"job_id":     in.JobUUID.String(),
			"format":     "pdf",
			"source_uri": in.SourceURI,
			"pages":      pageItems,
		}
		rawManifest := map[string]any{
			"space_id":   in.SpaceID.String(),
			"job_id":     in.JobUUID.String(),
			"format":     "pdf",
			"source_uri": in.SourceURI,
			"raw_format": strings.TrimSpace(in.OCRArtifacts.RawFormat),
			"raw":        rawItems,
		}
		pagesManifestBytes, _ = json.MarshalIndent(pagesManifest, "", "  ")
		rawManifestBytes, _ = json.MarshalIndent(rawManifest, "", "  ")
		if err := s.writeObject(pagesManifestKey, pagesManifestBytes); err != nil {
			return ArtifactBundleUpdate{}, err
		}
		if err := s.writeObject(rawManifestKey, rawManifestBytes); err != nil {
			return ArtifactBundleUpdate{}, err
		}
		pagesManifestURI = s.uri(pagesManifestKey)
		rawManifestURI = s.uri(rawManifestKey)
	}

	chunkBytes, _ := json.MarshalIndent(chunkManifest, "", "  ")
	vectorBytes, _ := json.MarshalIndent(vectorManifest, "", "  ")
	maskingBytes, _ := json.MarshalIndent(maskingReport, "", "  ")

	check := sha256.New()
	check.Write(chunkBytes)
	check.Write(vectorBytes)
	check.Write(maskingBytes)
	check.Write(pagesManifestBytes)
	check.Write(rawManifestBytes)
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
		ChunkManifestURI:    s.uri(chunkKey),
		VectorManifestURI:   s.uri(vectorKey),
		MaskingReportURI:    s.uri(maskingKey),
		OCRPageImagesURI:    pagesManifestURI,
		OCRRawManifestURI:   rawManifestURI,
		OCRSearchablePDFURI: searchablePDFURI,
		Checksum:            checksum,
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

// DeleteJobArtifacts removes local on-disk artifacts for a given job.
// It is a best-effort cleanup and only affects the filesystem-backed ArtifactStore.
func (s *ArtifactStore) DeleteJobArtifacts(spaceID uuid.UUID, jobUUID uuid.UUID) (bool, error) {
	if s == nil {
		return false, nil
	}
	if spaceID == uuid.Nil || jobUUID == uuid.Nil {
		return false, errors.New("spaceID/jobUUID is required")
	}
	// Keep consistent with `Write` objectPrefix: knowledge/<space>/<job>/
	dir := filepath.Join(s.baseDir, s.bucket, filepath.FromSlash(filepath.ToSlash(filepath.Join("knowledge", spaceID.String(), jobUUID.String()))))
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, os.RemoveAll(dir)
}
