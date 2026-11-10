package main

import (
	"context"
	"encoding/json"
	"errors"

	media "github.com/ArtisanCloud/PowerX/internal/service/media"
)

type derivativeBackfiller interface {
	BackfillImageDerivatives(context.Context, media.ImageDerivativeBackfillOptions, func(media.ImageDerivativeBackfillItem) error) (media.ImageDerivativeBackfillReport, error)
}

// Execution starts from candidate UUIDs and its own journal, never from the
// completed scan's after_id. This is orchestration only; TaskBus owns jobs.
func executeDerivativePlan(ctx context.Context, svc derivativeBackfiller, plan *derivativePlan, progress *derivativeProgress, options media.ImageDerivativeBackfillOptions, out *json.Encoder) error {
	var pending []media.ImageDerivativeBackfillItem
	acceptedBefore := 0
	for _, item := range plan.Candidates {
		if submissionAccepted(progress.latest[item.AssetUUID]) {
			acceptedBefore++
			continue
		}
		pending = append(pending, item)
	}
	if options.Limit > 0 && len(pending) > options.Limit {
		pending = pending[:options.Limit]
	}
	processed := 0
	var runErr error
	for start := 0; start < len(pending); start += options.BatchSize {
		if ctx.Err() != nil {
			runErr = ctx.Err()
			break
		}
		batch := pending[start:min(start+options.BatchSize, len(pending))]
		o := options
		o.TenantUUID, o.Apply, o.Limit = plan.Header.TenantUUID, true, 0
		o.AllTenants = plan.Header.Scope == derivativeScopeAll
		o.AfterID, o.ThroughID = plan.Header.AfterID, plan.Summary.ThroughID
		o.AssetUUIDs = nil
		o.Expected = map[string]media.ImageDerivativeBackfillExpectation{}
		approved := map[string]media.ImageDerivativeBackfillItem{}
		for _, item := range batch {
			o.AssetUUIDs = append(o.AssetUUIDs, item.AssetUUID)
			o.Expected[item.AssetUUID] = media.ImageDerivativeBackfillExpectation{TenantUUID: item.TenantUUID, SourceChecksum: item.SourceChecksum, Missing: item.Missing}
			approved[item.AssetUUID] = item
		}
		observed := map[string]bool{}
		emit := func(item media.ImageDerivativeBackfillItem) error {
			if err := progress.record(item); err != nil {
				return err
			}
			processed++
			return out.Encode(map[string]any{"kind": "asset", "result": item})
		}
		_, err := svc.BackfillImageDerivatives(ctx, o, func(item media.ImageDerivativeBackfillItem) error {
			original, found := approved[item.AssetUUID]
			if !found || observed[item.AssetUUID] || item.ID != original.ID || item.TenantUUID != original.TenantUUID || item.ThroughID != plan.Summary.ThroughID {
				return errors.New("MEDIA_BACKFILL_PLAN_RESULT_MISMATCH")
			}
			observed[item.AssetUUID] = true
			return emit(item)
		})
		if err != nil {
			runErr = err
			break
		}
		for _, item := range batch {
			if observed[item.AssetUUID] {
				continue
			}
			// Deleted or moved assets must remain visible, not disappear from a
			// successful submission report merely because the query returned none.
			item.ThroughID = plan.Summary.ThroughID
			item.Outcome, item.ErrorCode = "failed", "MEDIA_BACKFILL_ASSET_NOT_FOUND"
			if err := emit(item); err != nil {
				runErr = err
				break
			}
		}
		if runErr != nil {
			break
		}
	}
	counts := map[string]int{"submitted": 0, "active": 0, "skipped": 0, "failed": 0, "source_missing": 0}
	remaining := 0
	failed := []string{}
	sourceMissing := []string{}
	byTenant := map[string]map[string]int{}
	for _, item := range plan.Candidates {
		if byTenant[item.TenantUUID] == nil {
			byTenant[item.TenantUUID] = map[string]int{"candidates": 0, "remaining": 0}
		}
		byTenant[item.TenantUUID]["candidates"]++
		latest, found := progress.latest[item.AssetUUID]
		if !submissionAccepted(latest) {
			remaining++
			byTenant[item.TenantUUID]["remaining"]++
		}
		if found {
			counts[latest.Outcome]++
			byTenant[item.TenantUUID][latest.Outcome]++
		}
		if latest.Outcome == "failed" {
			failed = append(failed, item.AssetUUID)
		}
		if latest.Outcome == "source_missing" {
			sourceMissing = append(sourceMissing, item.AssetUUID)
		}
	}
	if err := out.Encode(map[string]any{"kind": "execution_summary", "plan_sha256": plan.SHA256, "environment_id": plan.Header.EnvironmentID, "scope": plan.Header.Scope, "tenant_uuid": plan.Header.TenantUUID, "tenant_count": plan.Summary.TenantCount, "scan_by_tenant": plan.ScanByTenant, "by_tenant": byTenant, "recipe": plan.Header.Recipe, "processed_this_run": processed, "accepted_before_run": acceptedBefore, "candidate_count": len(plan.Candidates), "scan_failed": plan.Summary.Failed, "scan_source_missing": plan.Summary.SourceMissing, "outcomes": counts, "remaining_candidates": remaining, "failed_assets": failed, "source_missing_assets": sourceMissing, "submission_complete": remaining == 0 && runErr == nil}); err != nil {
		return err
	}
	if runErr != nil {
		return runErr
	}
	if len(failed) > 0 || len(sourceMissing) > 0 {
		return errors.New("MEDIA_BACKFILL_APPLY_INCOMPLETE")
	}
	return nil
}
