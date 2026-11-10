package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	media "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/instanceidentity"
	"github.com/google/uuid"
)

const derivativePlanContract = "media_derivatives_backfill_plan.v2"
const derivativeProgressContract = "media_derivatives_backfill_progress.v2"

const derivativeScopeAll = "all_tenants"
const derivativeScopeTenant = "tenant"

type derivativePlanHeader struct {
	Contract      string    `json:"contract"`
	EnvironmentID string    `json:"environment_id"`
	Scope         string    `json:"scope"`
	TenantUUID    string    `json:"tenant_uuid,omitempty"`
	Recipe        string    `json:"recipe"`
	CreatedAt     time.Time `json:"created_at"`
	AssetUUIDs    []string  `json:"asset_uuids,omitempty"`
	AfterID       uint64    `json:"after_id"`
	ThroughID     uint64    `json:"through_id"`
	Limit         int       `json:"limit"`
}

type derivativePlan struct {
	Header       derivativePlanHeader
	Summary      media.ImageDerivativeBackfillReport
	Candidates   []media.ImageDerivativeBackfillItem
	ScanByTenant map[string]map[string]int
	SHA256       string
}

type derivativePlanLine struct {
	Kind      string                `json:"kind"`
	Plan      *derivativePlanHeader `json:"plan,omitempty"`
	Result    json.RawMessage       `json:"result,omitempty"`
	Namespace string                `json:"namespace,omitempty"`
	Complete  *bool                 `json:"complete,omitempty"`
}

func decodeMaintenanceJSON(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return errors.New("MEDIA_BACKFILL_FILE_INVALID")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("MEDIA_BACKFILL_FILE_INVALID")
	}
	return nil
}

func canonicalMaintenanceUUID(raw string) bool {
	id, err := uuid.Parse(raw)
	return err == nil && id != uuid.Nil && id.String() == raw
}

// A plan is a completed read-only scan, not an invocation payload. Truncated
// scans, duplicate records, unknown outcomes and inconsistent bounds fail closed.
func readDerivativePlan(path string) (*derivativePlan, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("MEDIA_BACKFILL_PLAN_UNREADABLE")
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
	}
	hash := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(f, hash))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	p := &derivativePlan{ScanByTenant: map[string]map[string]int{}}
	seen := map[string]bool{}
	header, summary := false, false
	last := uint64(0)
	counts := media.ImageDerivativeBackfillReport{}
	tenants := map[string]bool{}
	for scanner.Scan() {
		var line derivativePlanLine
		if summary || decodeMaintenanceJSON(scanner.Bytes(), &line) != nil {
			return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
		}
		switch line.Kind {
		case "plan":
			if header || line.Plan == nil || len(line.Result) != 0 || line.Complete != nil || line.Namespace != "" {
				return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
			}
			p.Header = *line.Plan
			h := p.Header
			validScope := h.Scope == derivativeScopeAll && h.TenantUUID == "" || h.Scope == derivativeScopeTenant && canonicalMaintenanceUUID(h.TenantUUID)
			if h.Contract != derivativePlanContract || !instanceidentity.Valid(h.EnvironmentID) || !validScope || h.Recipe != media.ImageDerivativeRecipe || h.CreatedAt.IsZero() || h.Limit < 0 || h.ThroughID > 0 && h.AfterID > h.ThroughID {
				return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
			}
			for _, asset := range h.AssetUUIDs {
				if !canonicalMaintenanceUUID(asset) || seen[asset] {
					return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
				}
				seen[asset] = true
			}
			seen = map[string]bool{}
			last = h.AfterID
			header = true
		case "asset":
			if !header || line.Plan != nil || line.Complete != nil || line.Namespace != "" {
				return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
			}
			var item media.ImageDerivativeBackfillItem
			if decodeMaintenanceJSON(line.Result, &item) != nil || !canonicalMaintenanceUUID(item.AssetUUID) || seen[item.AssetUUID] || item.ID <= last || item.ID > item.ThroughID {
				return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
			}
			validTenant := canonicalMaintenanceUUID(item.TenantUUID)
			if (!validTenant && !(p.Header.Scope == derivativeScopeAll && item.Outcome == "failed" && item.ErrorCode == "MEDIA_BACKFILL_TENANT_INVALID")) || (p.Header.Scope == derivativeScopeTenant && item.TenantUUID != p.Header.TenantUUID) {
				return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
			}
			if validTenant {
				tenants[item.TenantUUID] = true
				if p.ScanByTenant[item.TenantUUID] == nil {
					p.ScanByTenant[item.TenantUUID] = map[string]int{}
				}
				p.ScanByTenant[item.TenantUUID]["scanned"]++
				p.ScanByTenant[item.TenantUUID][item.Outcome]++
			}
			if p.Header.ThroughID > 0 && item.ThroughID != p.Header.ThroughID || counts.ThroughID > 0 && counts.ThroughID != item.ThroughID {
				return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
			}
			if len(p.Header.AssetUUIDs) > 0 {
				found := false
				for _, asset := range p.Header.AssetUUIDs {
					found = found || asset == item.AssetUUID
				}
				if !found {
					return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
				}
			}
			seen[item.AssetUUID] = true
			last = item.ID
			counts.ThroughID = item.ThroughID
			counts.Scanned++
			if len(item.Missing) > 0 {
				counts.Missing++
			}
			switch item.Outcome {
			case "would_enqueue":
				expected := media.ImageDerivativeBackfillExpectation{TenantUUID: item.TenantUUID, SourceChecksum: item.SourceChecksum, Missing: item.Missing}
				if !expected.Valid() || item.JobUUID != "" || item.ErrorCode != "" {
					return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
				}
				p.Candidates = append(p.Candidates, item)
			case "skipped":
				counts.Skipped++
			case "failed":
				counts.Failed++
			case "source_missing":
				counts.SourceMissing++
			default:
				return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
			}
		case "summary":
			if !header || line.Plan != nil || line.Complete == nil || !*line.Complete || line.Namespace != p.Header.EnvironmentID || decodeMaintenanceJSON(line.Result, &p.Summary) != nil {
				return nil, errors.New("MEDIA_BACKFILL_PLAN_INCOMPLETE")
			}
			s := p.Summary
			if !s.DryRun || s.Recipe != p.Header.Recipe || s.AfterID != last || s.ThroughID < last || s.Scanned != counts.Scanned || s.Missing != counts.Missing || s.Skipped != counts.Skipped || s.Failed != counts.Failed || s.SourceMissing != counts.SourceMissing || s.TenantCount != len(tenants) || s.Submitted != 0 || s.Active != 0 || (counts.ThroughID > 0 && s.ThroughID != counts.ThroughID) || (p.Header.ThroughID > 0 && s.ThroughID != p.Header.ThroughID) || (p.Header.Limit > 0 && s.Scanned > p.Header.Limit) {
				return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
			}
			summary = true
		default:
			return nil, errors.New("MEDIA_BACKFILL_PLAN_INVALID")
		}
	}
	if scanner.Err() != nil || !header || !summary {
		return nil, errors.New("MEDIA_BACKFILL_PLAN_INCOMPLETE")
	}
	p.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return p, nil
}

type derivativeProgressHeader struct {
	Contract      string `json:"contract"`
	PlanSHA256    string `json:"plan_sha256"`
	EnvironmentID string `json:"environment_id"`
	Scope         string `json:"scope"`
	TenantUUID    string `json:"tenant_uuid,omitempty"`
	Recipe        string `json:"recipe"`
}
type derivativeProgress struct {
	file   *os.File
	latest map[string]media.ImageDerivativeBackfillItem
}

func progressHeader(p *derivativePlan) derivativeProgressHeader {
	return derivativeProgressHeader{Contract: derivativeProgressContract, PlanSHA256: p.SHA256, EnvironmentID: p.Header.EnvironmentID, Scope: p.Header.Scope, TenantUUID: p.Header.TenantUUID, Recipe: p.Header.Recipe}
}

func openDerivativeProgress(path, planPath string, p *derivativePlan) (*derivativeProgress, error) {
	if sameMaintenanceFile(path, planPath) {
		return nil, errors.New("MEDIA_BACKFILL_PROGRESS_IS_PLAN")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, errors.New("MEDIA_BACKFILL_PROGRESS_UNWRITABLE")
	}
	progress := &derivativeProgress{file: f, latest: map[string]media.ImageDerivativeBackfillItem{}}
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
		}
	}()
	if err := lockDerivativeProgress(f); err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("MEDIA_BACKFILL_PROGRESS_INVALID")
	}
	if info.Size() == 0 {
		if err := progress.append(map[string]any{"kind": "execution", "execution": progressHeader(p)}); err != nil {
			return nil, err
		}
	} else {
		reader := bufio.NewReaderSize(f, 4096)
		offset := int64(0)
		header := false
		approved := map[string]media.ImageDerivativeBackfillItem{}
		for _, item := range p.Candidates {
			approved[item.AssetUUID] = item
		}
		for {
			raw, err := reader.ReadBytes('\n')
			if len(raw) > 1<<20 {
				return nil, errors.New("MEDIA_BACKFILL_PROGRESS_INVALID")
			}
			if err == io.EOF {
				if len(raw) > 0 {
					// Only discard an uncommitted tail after a verified header. A job
					// submitted before a crash is safely re-observed through ensure.
					if !header {
						return nil, errors.New("MEDIA_BACKFILL_PROGRESS_INVALID")
					}
					if err := f.Truncate(offset); err != nil {
						return nil, err
					}
				}
				break
			}
			if err != nil {
				return nil, err
			}
			var line struct {
				Kind      string                             `json:"kind"`
				Execution *derivativeProgressHeader          `json:"execution,omitempty"`
				Result    *media.ImageDerivativeBackfillItem `json:"result,omitempty"`
			}
			if decodeMaintenanceJSON(raw, &line) != nil {
				return nil, errors.New("MEDIA_BACKFILL_PROGRESS_INVALID")
			}
			if !header {
				if line.Kind != "execution" || line.Execution == nil || *line.Execution != progressHeader(p) || line.Result != nil {
					return nil, errors.New("MEDIA_BACKFILL_PROGRESS_PLAN_MISMATCH")
				}
				header = true
			} else {
				if line.Kind != "asset" || line.Result == nil || line.Execution != nil {
					return nil, errors.New("MEDIA_BACKFILL_PROGRESS_INVALID")
				}
				item := *line.Result
				a, found := approved[item.AssetUUID]
				if !found || item.ID != a.ID || item.TenantUUID != a.TenantUUID || item.ThroughID != p.Summary.ThroughID {
					return nil, errors.New("MEDIA_BACKFILL_PROGRESS_INVALID")
				}
				switch item.Outcome {
				case "submitted", "active":
					if !canonicalMaintenanceUUID(item.JobUUID) {
						return nil, errors.New("MEDIA_BACKFILL_PROGRESS_INVALID")
					}
				case "skipped", "failed", "source_missing":
				default:
					return nil, errors.New("MEDIA_BACKFILL_PROGRESS_INVALID")
				}
				progress.latest[item.AssetUUID] = item
			}
			offset += int64(len(raw))
		}
		if !header {
			return nil, errors.New("MEDIA_BACKFILL_PROGRESS_INVALID")
		}
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return nil, err
	}
	ok = true
	return progress, nil
}

func sameMaintenanceFile(a, b string) bool {
	x, err := os.Stat(a)
	if err != nil {
		return false
	}
	y, err := os.Stat(b)
	return err == nil && os.SameFile(x, y)
}
func (p *derivativeProgress) append(value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := p.file.Write(append(raw, '\n')); err != nil {
		return err
	}
	return p.file.Sync()
}
func (p *derivativeProgress) record(item media.ImageDerivativeBackfillItem) error {
	if err := p.append(map[string]any{"kind": "asset", "result": item}); err != nil {
		return err
	}
	p.latest[item.AssetUUID] = item
	return nil
}
func (p *derivativeProgress) close() { _ = p.file.Close() }
func submissionAccepted(item media.ImageDerivativeBackfillItem) bool {
	return item.Outcome == "submitted" || item.Outcome == "active" || item.Outcome == "skipped"
}

func createDerivativeReport(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("MEDIA_BACKFILL_REPORT_CREATE_FAILED: %w", err)
	}
	return f, nil
}
