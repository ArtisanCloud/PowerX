package storagegov

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const retentionCheckpointID = "c58cd741-57b3-5e1b-b85b-331d06d7ee59"

type retentionCheckpoint struct {
	Revision int64                     `json:"revision"`
	Cutoffs  caprepo.RetentionCutoffs  `json:"cutoffs"`
	Progress caprepo.RetentionProgress `json:"progress"`
}

func (s *Service) scanLedger() *repo.RetentionRunRepository {
	return repo.NewRetentionScanRepository(s.opts.DB, s.opts.Control.opts.Scope.ID())
}

func (s *Service) readRetentionCheckpoint(ctx context.Context, tx *gorm.DB) (*retentionCheckpoint, error) {
	row, err := s.scanLedger().Get(ctx, tx, retentionCheckpointID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var value retentionCheckpoint
	if row.Version != 1 || row.State != "checkpoint" || row.ActorID != 0 || json.Unmarshal(row.Document, &value) != nil ||
		value.Revision < 1 || !value.Progress.Valid() || value.Cutoffs.Success.IsZero() || value.Cutoffs.Failure.IsZero() ||
		value.Cutoffs.Pending.IsZero() || value.Cutoffs.Quarantine.IsZero() || value.Cutoffs.Failure.After(value.Cutoffs.Success) ||
		value.Cutoffs.Limit < 1 || value.Cutoffs.Limit > 1000 {
		return nil, controlError(503, "DATABASE_RETENTION_CHECKPOINT_INVALID")
	}
	return &value, nil
}

func (s *Service) retentionScanWindow(ctx context.Context, tx *gorm.DB, p DatabaseRetentionPolicy, now time.Time) (caprepo.RetentionWindow, caprepo.RetentionCutoffs, bool, error) {
	old, err := s.readRetentionCheckpoint(ctx, tx)
	if err != nil {
		return caprepo.RetentionWindow{}, caprepo.RetentionCutoffs{}, false, err
	}
	r := caprepo.NewRetentionRepository(tx)
	cutoffs := s.retentionCutoffs(p, now)
	var after, high caprepo.RetentionPosition
	if old != nil && old.Revision == p.Revision && !old.Progress.Complete {
		if old.Cutoffs.Limit != p.BatchLimit || old.Cutoffs.AdoptLegacy != p.AdoptLegacy {
			return caprepo.RetentionWindow{}, cutoffs, true, controlError(503, "DATABASE_RETENTION_CHECKPOINT_INVALID")
		}
		after, high, cutoffs = old.Progress.Next, old.Progress.Window.Highwater, old.Cutoffs
	} else {
		high, err = r.ScanHighwater(ctx)
		if err != nil {
			return caprepo.RetentionWindow{}, cutoffs, old != nil, err
		}
	}
	w, err := r.ScanWindow(ctx, after, high)
	return w, cutoffs, old != nil, err
}

func (s *Service) saveRetentionCheckpoint(ctx context.Context, tx *gorm.DB, run DatabaseRetentionRun, exists bool) error {
	raw, err := json.Marshal(retentionCheckpoint{run.Revision, *run.Cutoffs, *run.Scan})
	if err != nil {
		return err
	}
	row := &m.CleanupRun{RunID: retentionCheckpointID, Version: 1, State: "checkpoint", ActorID: 0, ExpiresAt: run.At.AddDate(0, 0, 30), Document: datatypes.JSON(raw)}
	if exists {
		return s.scanLedger().ReplaceCheckpoint(ctx, tx, row)
	}
	return s.scanLedger().Create(ctx, tx, row)
}

func retentionExecutionError(err error) error {
	if caprepo.IsRetentionQueryTimeout(err) {
		return controlError(503, "DATABASE_RETENTION_QUERY_TIMEOUT")
	}
	return err
}
