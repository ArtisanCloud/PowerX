package storagegov

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const datasetMaxExamined = 2048

// DatasetRetentionBounds constrains administrator-proposed retention.
type DatasetRetentionBounds struct {
	MinimumDays   int `json:"minimum_days"`
	MaximumDays   int `json:"maximum_days"`
	SuggestedDays int `json:"suggested_days"`
}

func (b DatasetRetentionBounds) Validate() error {
	if b.MinimumDays < 1 || b.MaximumDays > 3650 || b.SuggestedDays < b.MinimumDays || b.SuggestedDays > b.MaximumDays {
		return errors.New("invalid dataset retention bounds")
	}
	return nil
}

func parseDatasetUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil || id.String() != value {
		return uuid.Nil, errors.New("invalid dataset UUID")
	}
	return id, nil
}

// DatasetProtectionCount classifies one group of protected root records.
type DatasetProtectionCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// DatasetPreviewCounts counts root records examined within one granted range.
type DatasetPreviewCounts struct {
	Coverage          string                   `json:"coverage"`
	ExaminedCount     int                      `json:"examined_count"`
	CandidateCount    int                      `json:"candidate_count"`
	ProtectedByReason []DatasetProtectionCount `json:"protected_by_reason"`
}

// Validate checks relational constraints against the claimed scan limit.
func (p DatasetPreviewCounts) Validate(maxExamined int) error {
	if maxExamined < 1 || maxExamined > datasetMaxExamined || p.ExaminedCount < 0 || p.ExaminedCount > maxExamined ||
		p.CandidateCount < 0 || p.CandidateCount > p.ExaminedCount || (p.Coverage != "complete" && p.Coverage != "bounded") ||
		p.ProtectedByReason == nil || len(p.ProtectedByReason) > 7 {
		return errors.New("invalid dataset preview counts")
	}
	remaining := p.ExaminedCount - p.CandidateCount
	seen := make(map[string]bool, len(p.ProtectedByReason))
	for _, item := range p.ProtectedByReason {
		switch item.Reason {
		case "referenced", "active", "retryable", "unknown_eligibility", "retained_evidence", "latest_effective", "incomplete_record":
		default:
			return errors.New("unknown dataset protection reason")
		}
		if seen[item.Reason] || item.Count < 1 || item.Count > remaining {
			return errors.New("invalid dataset protected counts")
		}
		seen[item.Reason] = true
		remaining -= item.Count
	}
	return nil
}

// DatasetPreviewTask identifies a Core-persisted preview task and its consumer.
// The queue topic and client payload are not sources of these fields.
type DatasetPreviewTask struct {
	RequestUUID  uuid.UUID
	TaskID       string
	DeploymentID string
	TenantUUID   uuid.UUID
	PluginID     string
	Installation string
	SubscriberID string
}

// DatasetPreviewLease is resolved by Core from the actual TaskBus delivery.
type DatasetPreviewLease struct {
	Task        DatasetPreviewTask
	ReceiptUUID uuid.UUID
	Fence       uint64
	ExpiresAt   time.Time
}

// ValidateDatasetPreviewLease verifies the authoritative task association and
// the active receipt. Exact replays of committed results are reconciled separately.
func ValidateDatasetPreviewLease(coreTask DatasetPreviewTask, lease DatasetPreviewLease, receiptUUID uuid.UUID, fence uint64, now time.Time) error {
	if coreTask.RequestUUID == uuid.Nil || coreTask.TaskID == "" || coreTask.DeploymentID == "" || coreTask.TenantUUID == uuid.Nil ||
		coreTask.PluginID == "" || coreTask.Installation == "" || coreTask.SubscriberID == "" || now.IsZero() ||
		receiptUUID == uuid.Nil || fence < 1 || fence > 1<<53-1 || coreTask != lease.Task ||
		receiptUUID != lease.ReceiptUUID || fence != lease.Fence || !now.Before(lease.ExpiresAt) {
		return errors.New("dataset preview task lease mismatch")
	}
	return nil
}
