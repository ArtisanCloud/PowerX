package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"gorm.io/gorm"
)

const datasetExecutionMaxRequestBytes = 4*archive.MaxDatasetArchiveBytes/3 + 8192

func (s *Service) datasetArchiveTarget(ctx context.Context, id string) (archive.Target, error) {
	if s.recoveryArchives == nil {
		return archive.Target{}, controlError(503, "DATASET_ARCHIVE_TARGET_UNAVAILABLE")
	}
	return s.recoveryArchives.DatasetArchiveTarget(ctx, id)
}

func (r datasetRunDocument) archiveBinding() archive.DatasetArchiveBinding {
	return archive.DatasetArchiveBinding{
		TargetID: r.ArchiveTargetID, TargetRevision: r.ArchiveTargetRev, TargetFingerprint: r.ArchiveTargetDigest,
		DatasetUUID: r.Run.DatasetUUID, TenantUUID: r.Run.TenantUUID, BatchUUID: r.Run.RequestUUID,
	}
}

type datasetArchiveSubmission struct {
	Report datasetExecutionReport `json:"report"`
	Data   []byte                 `json:"archive_base64"`
}

func validDatasetArchive(data []byte, rows int) bool {
	if len(data) == 0 || !utf8.Valid(data) || len(data) > archive.MaxDatasetArchiveBytes || data[len(data)-1] != '\n' || rows < 1 || rows > datasetWriteBatchLimit {
		return false
	}
	records := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(records) != rows {
		return false
	}
	for _, raw := range records {
		record := bytes.TrimSpace(raw)
		if len(record) == 0 || record[0] != '{' || !json.Valid(record) {
			return false
		}
	}
	return true
}

func (s *Service) archiveDatasetExecution(ctx context.Context, raw json.RawMessage, tenant, plugin, subject string) (map[string]any, error) {
	var input datasetArchiveSubmission
	if !decodeDatasetHost(raw, &input) || !input.Report.valid() || input.Report.Outcome != "committed" ||
		!validDatasetArchive(input.Data, *input.Report.AffectedCount) {
		return nil, controlError(400, "DATASET_ARCHIVE_CONTENT_INVALID")
	}
	report := input.Report
	doc, err := s.ownedDatasetRun(ctx, report.RequestUUID, tenant, plugin, subject)
	if err != nil {
		return nil, err
	}
	if doc.Run.Action != "archive" || s.recoveryArchives == nil {
		return nil, controlError(409, "DATASET_EXECUTION_ACTION_MISMATCH")
	}
	bodyDigest := controlDigest(input.Data)
	reportBytes, _ := json.Marshal(report)
	reportDigest := controlDigest(reportBytes)
	// The transfer follows an immutable database intent. Do not hold locks
	// while waiting on the network. No source data changes during archive.
	err = s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, current, err := s.loadDatasetRun(ctx, tx, report.RequestUUID, true)
		if err != nil {
			return err
		}
		if err := s.checkDatasetResultGrant(current, report); err != nil {
			return err
		}
		if current.ArchiveContentDigest != "" {
			if current.ArchiveContentDigest != bodyDigest || current.ArchiveReportDigest != reportDigest {
				return controlError(409, "DATASET_EXECUTION_REPORT_CONFLICT")
			}
			doc = current
			return nil
		}
		if current.ReportDigest != "" {
			return controlError(409, "DATASET_EXECUTION_REPORT_CONFLICT")
		}
		current.ArchiveContentDigest, current.ArchiveReportDigest = bodyDigest, reportDigest
		doc = current
		return storeDatasetWriteDocument(ctx, tx, row, current.Run.State, current)
	})
	if err != nil {
		return nil, err
	}
	if doc.ArchiveObject != nil {
		return s.recordDatasetResult(ctx, doc, report, doc.ArchiveObject)
	}
	run, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	object, err := s.recoveryArchives.PutDatasetArchive(run, doc.archiveBinding(), input.Data)
	if err != nil {
		return nil, err
	}
	return s.recordDatasetResult(ctx, doc, report, &object)
}

// DatasetArchive returns only the verified content of this Root-selected batch.
func (s *Service) DatasetArchive(ctx context.Context, actor uint64, dataset, request string) ([]byte, error) {
	r, err := s.DatasetExecution(ctx, actor, dataset, request)
	if err != nil {
		return nil, err
	}
	_, doc, err := s.loadDatasetRun(ctx, s.opts.DB, request, false)
	if err != nil {
		return nil, err
	}
	if s.recoveryArchives == nil || r.Action != "archive" || r.State != "committed" || doc.ArchiveObject == nil {
		return nil, controlError(409, "DATASET_ARCHIVE_UNAVAILABLE")
	}
	return s.recoveryArchives.ReadDatasetArchive(ctx, doc.archiveBinding(), *doc.ArchiveObject)
}
