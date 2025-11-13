package provider_registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"

	driver "github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
)

const (
	defaultArtifactBucket = "agent"
	defaultArtifactPrefix = "providers"
	defaultArtifactScheme = "minio"

	validationStatusUnknown = "unknown"
	validationStatusPass    = "pass"
	validationStatusFail    = "fail"
)

// ValidationArtifactOptions wires MinIO/S3 dependencies for validation artifacts.
type ValidationArtifactOptions struct {
	MediaManager *mediamgr.MediaManager
	Driver       string
	Bucket       string
	Prefix       string
	URIScheme    string
}

type validationArtifactStore struct {
	manager   *mediamgr.MediaManager
	driver    string
	bucket    string
	prefix    string
	uriScheme string
	clock     func() time.Time
}

type validationArtifactRecord struct {
	URI       string
	Bucket    string
	ObjectKey string
	Size      int64
	Checksum  string
	StoredAt  time.Time
}

func newValidationArtifactStore(opts ValidationArtifactOptions, clock func() time.Time) *validationArtifactStore {
	if opts.MediaManager == nil {
		return nil
	}
	driverName := strings.TrimSpace(opts.Driver)
	bucket := strings.TrimSpace(opts.Bucket)
	if bucket == "" {
		bucket = defaultArtifactBucket
	}
	prefix := strings.Trim(strings.TrimSpace(opts.Prefix), "/")
	if prefix == "" {
		prefix = defaultArtifactPrefix
	}
	scheme := strings.TrimSpace(opts.URIScheme)
	if scheme == "" {
		scheme = defaultArtifactScheme
	}
	if clock == nil {
		clock = time.Now
	}
	return &validationArtifactStore{
		manager:   opts.MediaManager,
		driver:    driverName,
		bucket:    bucket,
		prefix:    prefix,
		uriScheme: scheme,
		clock:     clock,
	}
}

func (s *validationArtifactStore) Save(ctx context.Context, providerID uuid.UUID, suite string, payload []byte) (*validationArtifactRecord, error) {
	if s == nil || s.manager == nil {
		return nil, fmt.Errorf("validation artifact store is not configured")
	}
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	now := s.clock().UTC()
	suiteSlug := sanitizeSuite(suite)
	objectKey := path.Join(s.prefix, providerID.String(), fmt.Sprintf("%s_%s.json", now.Format("20060102T150405Z"), suiteSlug))
	hash := sha256.Sum256(payload)
	reader := bytes.NewReader(payload)

	_, err := s.manager.Put(ctx, s.driver, driver.PutObjectInput{
		Bucket:      s.bucket,
		ObjectKey:   objectKey,
		Body:        reader,
		Size:        int64(len(payload)),
		ContentType: "application/json",
		Metadata: map[string]string{
			"provider_id": providerID.String(),
			"suite":       suiteSlug,
		},
		Overwrite: true,
	})
	if err != nil {
		return nil, fmt.Errorf("write validation artifact failed: %w", err)
	}
	return &validationArtifactRecord{
		URI:       fmt.Sprintf("%s://%s/%s", s.uriScheme, s.bucket, objectKey),
		Bucket:    s.bucket,
		ObjectKey: objectKey,
		Size:      int64(len(payload)),
		Checksum:  hex.EncodeToString(hash[:]),
		StoredAt:  now,
	}, nil
}

func sanitizeSuite(suite string) string {
	slug := strings.TrimSpace(strings.ToLower(suite))
	if slug == "" {
		slug = "full"
	}
	slug = strings.ReplaceAll(slug, " ", "_")
	return slug
}

// ValidationReport captures the Ops validator summary payload.
type ValidationReport struct {
	ProviderID  string                  `json:"providerId"`
	Suite       string                  `json:"suite"`
	GeneratedAt string                  `json:"generatedAt"`
	Stats       ValidationReportStats   `json:"stats"`
	Results     []ValidationReportEntry `json:"results"`
}

// ValidationReportStats reflects aggregate counts from validator output.
type ValidationReportStats struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

// ValidationReportEntry captures per-test result.
type ValidationReportEntry struct {
	Name      string `json:"name"`
	Modality  string `json:"modality"`
	Success   bool   `json:"success"`
	Status    int    `json:"status"`
	LatencyMs int    `json:"latencyMs"`
	Endpoint  string `json:"endpoint"`
	Error     string `json:"error"`
}

func (r *ValidationReport) Normalize(providerID uuid.UUID, suite string, clock func() time.Time) {
	if r == nil {
		return
	}
	if strings.TrimSpace(r.ProviderID) == "" && providerID != uuid.Nil {
		r.ProviderID = providerID.String()
	}
	if strings.TrimSpace(r.Suite) == "" {
		r.Suite = suite
	}
	if clock != nil && strings.TrimSpace(r.GeneratedAt) == "" {
		r.GeneratedAt = clock().UTC().Format(time.RFC3339)
	}
	r.ensureStats()
}

func (r *ValidationReport) ensureStats() {
	if r == nil {
		return
	}
	if r.Stats.Total == 0 && len(r.Results) > 0 {
		total := len(r.Results)
		passed := 0
		for _, res := range r.Results {
			if res.Success {
				passed++
			}
		}
		r.Stats.Total = total
		r.Stats.Passed = passed
		r.Stats.Failed = total - passed
	}
}

func (r *ValidationReport) Status() string {
	if r == nil {
		return validationStatusUnknown
	}
	if r.Passed() {
		return validationStatusPass
	}
	return validationStatusFail
}

func (r *ValidationReport) Passed() bool {
	if r == nil {
		return true
	}
	if r.Stats.Failed > 0 {
		return false
	}
	for _, res := range r.Results {
		if !res.Success {
			return false
		}
	}
	return true
}

func (r *ValidationReport) StatsMap() map[string]int {
	if r == nil {
		return nil
	}
	return map[string]int{
		"total":  r.Stats.Total,
		"passed": r.Stats.Passed,
		"failed": r.Stats.Failed,
	}
}

func (r *ValidationReport) MarshalJSONBytes() ([]byte, error) {
	if r == nil {
		return json.Marshal(map[string]any{})
	}
	type alias ValidationReport
	return json.Marshal((*alias)(r))
}

func artifactMetaToJSON(record *validationArtifactRecord, report *ValidationReport, status string) datatypes.JSONMap {
	if record == nil {
		return nil
	}
	meta := datatypes.JSONMap{
		"uri":        record.URI,
		"bucket":     record.Bucket,
		"object_key": record.ObjectKey,
		"size":       record.Size,
		"checksum":   record.Checksum,
		"stored_at":  record.StoredAt.UTC().Format(time.RFC3339),
		"status":     status,
	}
	if report != nil {
		meta["suite"] = report.Suite
		meta["generated_at"] = report.GeneratedAt
		if stats := report.StatsMap(); stats != nil {
			meta["stats"] = stats
		}
		meta["provider_id"] = report.ProviderID
	}
	return meta
}
