package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	media "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func maintenancePlanFixture(t *testing.T) (string, *derivativePlan) {
	t.Helper()
	header := derivativePlanHeader{Contract: derivativePlanContract, EnvironmentID: "maintenance-test", Scope: derivativeScopeTenant, TenantUUID: uuid.NewString(), Recipe: media.ImageDerivativeRecipe, CreatedAt: time.Now().UTC(), Limit: 100}
	path := filepath.Join(t.TempDir(), "scan.jsonl")
	f, err := createDerivativeReport(path)
	require.NoError(t, err)
	e := json.NewEncoder(f)
	require.NoError(t, e.Encode(map[string]any{"kind": "plan", "plan": header}))
	for i := 1; i <= 3; i++ {
		item := media.ImageDerivativeBackfillItem{ID: uint64(i * 10), ThroughID: 30, AssetUUID: uuid.NewString(), TenantUUID: header.TenantUUID, Outcome: "would_enqueue", Missing: []string{"thumbnail_64"}, SourceChecksum: strings.Repeat("a", 64)}
		if i == 3 {
			item.Outcome, item.Missing = "skipped", nil
		}
		require.NoError(t, e.Encode(map[string]any{"kind": "asset", "result": item}))
	}
	require.NoError(t, e.Encode(map[string]any{"kind": "summary", "namespace": header.EnvironmentID, "complete": true, "result": media.ImageDerivativeBackfillReport{DryRun: true, Recipe: header.Recipe, TenantCount: 1, Scanned: 3, Missing: 2, Skipped: 1, AfterID: 30, ThroughID: 30}}))
	require.NoError(t, f.Close())
	p, err := readDerivativePlan(path)
	require.NoError(t, err)
	return path, p
}

func TestDerivativePlanRejectsIncompleteOrChangedScope(t *testing.T) {
	path, plan := maintenancePlanFixture(t)
	require.Len(t, plan.Candidates, 2)
	require.EqualValues(t, 30, plan.Summary.AfterID)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	for _, mutation := range []string{"no_footer", "interrupted", "not_dry", "duplicate", "bad_checksum", "unknown_spec", "wrong_count", "wrong_namespace", "extra_record", "unknown_field"} {
		t.Run(mutation, func(t *testing.T) {
			parts := make([][]byte, len(lines))
			for i := range lines {
				parts[i] = append([]byte(nil), lines[i]...)
			}
			last := len(parts) - 1
			switch mutation {
			case "no_footer":
				parts = parts[:last]
			case "interrupted":
				parts[last] = bytes.ReplaceAll(parts[last], []byte(`"complete":true`), []byte(`"complete":false`))
			case "not_dry":
				parts[last] = bytes.ReplaceAll(parts[last], []byte(`"dry_run":true`), []byte(`"dry_run":false`))
			case "duplicate":
				parts[2] = parts[1]
			case "bad_checksum":
				parts[1] = bytes.ReplaceAll(parts[1], []byte(strings.Repeat("a", 64)), []byte("invalid"))
			case "unknown_spec":
				parts[1] = bytes.ReplaceAll(parts[1], []byte(`"thumbnail_64"`), []byte(`"origin"`))
			case "wrong_count":
				parts[last] = bytes.ReplaceAll(parts[last], []byte(`"scanned":3`), []byte(`"scanned":2`))
			case "wrong_namespace":
				parts[last] = bytes.ReplaceAll(parts[last], []byte("maintenance-test"), []byte("another-environment"))
			case "extra_record":
				parts = append(parts, parts[1])
			case "unknown_field":
				parts[0] = bytes.Replace(parts[0], []byte(`"kind":"plan"`), []byte(`"kind":"plan","endpoint":"/arbitrary"`), 1)
			}
			file := filepath.Join(t.TempDir(), "invalid.jsonl")
			require.NoError(t, os.WriteFile(file, bytes.Join(parts, []byte("\n")), 0600))
			_, err := readDerivativePlan(file)
			require.Error(t, err)
		})
	}
	_, err = createDerivativeReport(path)
	require.Error(t, err)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, raw, after)
}

type maintenanceBackfillStub struct {
	call func(context.Context, media.ImageDerivativeBackfillOptions, func(media.ImageDerivativeBackfillItem) error) (media.ImageDerivativeBackfillReport, error)
}

func (s maintenanceBackfillStub) BackfillImageDerivatives(ctx context.Context, o media.ImageDerivativeBackfillOptions, report func(media.ImageDerivativeBackfillItem) error) (media.ImageDerivativeBackfillReport, error) {
	return s.call(ctx, o, report)
}

func TestDerivativePlanExecutionUsesOwnProgressAndExactCandidates(t *testing.T) {
	path, plan := maintenancePlanFixture(t)
	progressPath := path + ".progress.jsonl"
	progress, err := openDerivativeProgress(progressPath, path, plan)
	require.NoError(t, err)
	defer progress.close()
	calls := 0
	stub := maintenanceBackfillStub{call: func(ctx context.Context, o media.ImageDerivativeBackfillOptions, report func(media.ImageDerivativeBackfillItem) error) (media.ImageDerivativeBackfillReport, error) {
		require.True(t, o.Apply)
		require.Equal(t, plan.Header.TenantUUID, o.TenantUUID)
		require.Zero(t, o.AfterID, "scan's completed cursor must never skip apply candidates")
		require.EqualValues(t, 30, o.ThroughID)
		require.Len(t, o.AssetUUIDs, 1)
		expected := plan.Candidates[calls]
		require.Equal(t, []string{expected.AssetUUID}, o.AssetUUIDs)
		require.Equal(t, media.ImageDerivativeBackfillExpectation{TenantUUID: expected.TenantUUID, SourceChecksum: expected.SourceChecksum, Missing: expected.Missing}, o.Expected[expected.AssetUUID])
		calls++
		expected.Outcome, expected.JobUUID = "submitted", uuid.NewString()
		return media.ImageDerivativeBackfillReport{}, report(expected)
	}}
	options := media.ImageDerivativeBackfillOptions{BatchSize: 1, Concurrency: 1, Limit: 1}
	for i := 0; i < 3; i++ {
		require.NoError(t, executeDerivativePlan(context.Background(), stub, plan, progress, options, json.NewEncoder(io.Discard)))
	}
	require.Equal(t, 2, calls)
	require.Len(t, progress.latest, 2)
	progress.close()
	progress, err = openDerivativeProgress(progressPath, path, plan)
	require.NoError(t, err)
	defer progress.close()
	require.NoError(t, executeDerivativePlan(context.Background(), stub, plan, progress, options, json.NewEncoder(io.Discard)))
	require.Equal(t, 2, calls)
}

func TestDerivativeProgressLocksBindsAndRecoversInterruptedTail(t *testing.T) {
	path, plan := maintenancePlanFixture(t)
	progressPath := path + ".progress.jsonl"
	p, err := openDerivativeProgress(progressPath, path, plan)
	require.NoError(t, err)
	_, err = openDerivativeProgress(progressPath, path, plan)
	require.ErrorContains(t, err, "MEDIA_BACKFILL_PROGRESS_LOCKED")
	item := plan.Candidates[0]
	item.Outcome, item.JobUUID = "submitted", uuid.NewString()
	require.NoError(t, p.record(item))
	require.NoError(t, p.file.Close())
	f, err := os.OpenFile(progressPath, os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	_, err = f.WriteString(`{"kind":"asset","result":`)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	p, err = openDerivativeProgress(progressPath, path, plan)
	require.NoError(t, err)
	require.Len(t, p.latest, 1)
	p.close()
	other := *plan
	other.SHA256 = strings.Repeat("0", 64)
	_, err = openDerivativeProgress(progressPath, path, &other)
	require.ErrorContains(t, err, "MEDIA_BACKFILL_PROGRESS_PLAN_MISMATCH")
	_, err = openDerivativeProgress(path, path, plan)
	require.ErrorContains(t, err, "MEDIA_BACKFILL_PROGRESS_IS_PLAN")
	raw, err := os.ReadFile(progressPath)
	require.NoError(t, err)
	require.True(t, bytes.HasSuffix(raw, []byte("\n")))
	info, err := os.Stat(progressPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestDerivativeApplyPersistsFailuresAndReportsAbsentAssets(t *testing.T) {
	path, plan := maintenancePlanFixture(t)
	p, err := openDerivativeProgress(path+".progress.jsonl", path, plan)
	require.NoError(t, err)
	defer p.close()
	calls := 0
	stub := maintenanceBackfillStub{call: func(ctx context.Context, o media.ImageDerivativeBackfillOptions, report func(media.ImageDerivativeBackfillItem) error) (media.ImageDerivativeBackfillReport, error) {
		calls++
		item := plan.Candidates[0]
		item.Outcome, item.ErrorCode = "failed", "MEDIA_BACKFILL_PLAN_CHANGED"
		return media.ImageDerivativeBackfillReport{}, report(item) // second UUID disappeared
	}}
	var output bytes.Buffer
	err = executeDerivativePlan(context.Background(), stub, plan, p, media.ImageDerivativeBackfillOptions{BatchSize: 2, Concurrency: 1}, json.NewEncoder(&output))
	require.ErrorContains(t, err, "MEDIA_BACKFILL_APPLY_INCOMPLETE")
	require.Equal(t, "MEDIA_BACKFILL_ASSET_NOT_FOUND", p.latest[plan.Candidates[1].AssetUUID].ErrorCode)
	require.Contains(t, output.String(), `"failed_assets":[`)
	require.Contains(t, output.String(), `"submission_complete":false`)
	require.Equal(t, 1, calls)
}

func TestDerivativeApplyCancellationKeepsAcceptedPrefixForResume(t *testing.T) {
	path, plan := maintenancePlanFixture(t)
	p, err := openDerivativeProgress(path+".progress.jsonl", path, plan)
	require.NoError(t, err)
	defer p.close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stub := maintenanceBackfillStub{call: func(ctx context.Context, o media.ImageDerivativeBackfillOptions, report func(media.ImageDerivativeBackfillItem) error) (media.ImageDerivativeBackfillReport, error) {
		item := plan.Candidates[0]
		item.Outcome, item.JobUUID = "submitted", uuid.NewString()
		require.NoError(t, report(item))
		cancel()
		return media.ImageDerivativeBackfillReport{}, ctx.Err()
	}}
	err = executeDerivativePlan(ctx, stub, plan, p, media.ImageDerivativeBackfillOptions{BatchSize: 2, Concurrency: 1}, json.NewEncoder(io.Discard))
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, p.latest, 1)
	stub.call = func(ctx context.Context, o media.ImageDerivativeBackfillOptions, report func(media.ImageDerivativeBackfillItem) error) (media.ImageDerivativeBackfillReport, error) {
		require.Equal(t, []string{plan.Candidates[1].AssetUUID}, o.AssetUUIDs)
		item := plan.Candidates[1]
		item.Outcome, item.JobUUID = "submitted", uuid.NewString()
		return media.ImageDerivativeBackfillReport{}, report(item)
	}
	require.NoError(t, executeDerivativePlan(context.Background(), stub, plan, p, media.ImageDerivativeBackfillOptions{BatchSize: 2, Concurrency: 1}, json.NewEncoder(io.Discard)))
	require.Len(t, p.latest, 2)
}

func TestDerivativePlanFlagsRejectBeforeConfigurationOrConnection(t *testing.T) {
	path, plan := maintenancePlanFixture(t)
	for _, args := range [][]string{
		{"--plan", path},
		{"--plan", path, "--dry-run=false"},
		{"--plan", path, "--dry-run=false", "--confirm", "--after-id", "30"},
		{"--plan", path, "--dry-run=false", "--confirm", "--tenant", uuid.NewString()},
		{"--tenant", plan.Header.TenantUUID, "--dry-run=false", "--report", filepath.Join(t.TempDir(), "bad.jsonl")},
	} {
		err := runDerivativeBackfill(append(args, "--config", "/nonexistent/core.yaml"))
		require.Error(t, err)
		require.NotContains(t, err.Error(), "无法加载目标")
	}
}

func TestMediaMakeEntriesForwardLiteralPathsAndRequireConfirmation(t *testing.T) {
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	dir := t.TempDir()
	tool := filepath.Join(dir, "fake tool")
	capture := filepath.Join(dir, "argv")
	marker := filepath.Join(dir, "unexpected-execution")
	require.NoError(t, os.WriteFile(tool, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$MEDIA_TEST_ARGS\"\n"), 0700))
	config := filepath.Join(dir, "config `touch unexpected-execution`; file.yaml")
	report := filepath.Join(dir, "scan report.jsonl")
	command := exec.Command("make", "--no-print-directory", "media-derivatives-scan", "CONFIG="+config, "TENANT_UUID="+uuid.NewString(), "REPORT="+report, "MEDIA_TOOL="+tool)
	command.Dir = root
	command.Env = append(os.Environ(), "MEDIA_TEST_ARGS="+capture)
	raw, err := command.CombinedOutput()
	require.NoError(t, err, string(raw))
	args, err := os.ReadFile(capture)
	require.NoError(t, err)
	require.Contains(t, string(args), "--config\n"+config+"\n")
	require.Contains(t, string(args), "--report\n"+report+"\n")
	// The same entry must default to all tenants and the complete snapshot,
	// rather than require a tenant or silently stop after 100 records.
	command = exec.Command("make", "--no-print-directory", "media-derivatives-scan", "CONFIG="+config, "REPORT="+report, "MEDIA_TOOL="+tool)
	command.Dir = root
	command.Env = append(os.Environ(), "MEDIA_TEST_ARGS="+capture)
	raw, err = command.CombinedOutput()
	require.NoError(t, err, string(raw))
	args, err = os.ReadFile(capture)
	require.NoError(t, err)
	require.NotContains(t, string(args), "--tenant\n")
	require.Contains(t, string(args), "--limit\n0\n")
	_, err = os.Stat(marker)
	require.True(t, errors.Is(err, os.ErrNotExist))
	_, err = os.Stat(filepath.Join(root, "backend/unexpected-execution"))
	require.True(t, errors.Is(err, os.ErrNotExist))
	_, err = os.Stat(filepath.Join(root, "unexpected-execution"))
	require.True(t, errors.Is(err, os.ErrNotExist))
	require.NoError(t, os.Remove(capture))
	command = exec.Command("make", "--no-print-directory", "media-derivatives-apply", "PLAN="+report, "MEDIA_TOOL="+tool)
	command.Dir = root
	command.Env = append(os.Environ(), "MEDIA_TEST_ARGS="+capture)
	raw, err = command.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(raw), "MEDIA_BACKFILL_CONFIRM_REQUIRED")
	_, err = os.Stat(capture)
	require.True(t, errors.Is(err, os.ErrNotExist))
	command = exec.Command("make", "--no-print-directory", "media-derivatives-apply", "CONFIG="+config, "PLAN="+report, "CONFIRM=1", "MEDIA_TOOL="+tool)
	command.Dir = root
	command.Env = append(os.Environ(), "MEDIA_TEST_ARGS="+capture)
	raw, err = command.CombinedOutput()
	require.NoError(t, err, string(raw))
	args, err = os.ReadFile(capture)
	require.NoError(t, err)
	require.Contains(t, string(args), "--plan\n"+report+"\n")
	require.Contains(t, string(args), "--dry-run=false\n--confirm\n")
	require.NotContains(t, string(args), "--after-id")
}

func TestAllTenantPlanPreservesEachRecipientTenantAndScope(t *testing.T) {
	path, plan := maintenancePlanFixture(t)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var lines []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		var value map[string]any
		require.NoError(t, json.Unmarshal(line, &value))
		lines = append(lines, value)
	}
	header := lines[0]["plan"].(map[string]any)
	header["scope"] = derivativeScopeAll
	delete(header, "tenant_uuid")
	otherTenant := uuid.NewString()
	lines[2]["result"].(map[string]any)["tenant_uuid"] = otherTenant
	lines[len(lines)-1]["result"].(map[string]any)["tenant_count"] = 2
	global := filepath.Join(t.TempDir(), "global.jsonl")
	f, err := createDerivativeReport(global)
	require.NoError(t, err)
	for _, line := range lines {
		require.NoError(t, json.NewEncoder(f).Encode(line))
	}
	require.NoError(t, f.Close())
	plan, err = readDerivativePlan(global)
	require.NoError(t, err)
	require.Equal(t, 2, plan.Summary.TenantCount)
	p, err := openDerivativeProgress(global+".progress.jsonl", global, plan)
	require.NoError(t, err)
	defer p.close()
	stub := maintenanceBackfillStub{call: func(ctx context.Context, o media.ImageDerivativeBackfillOptions, emit func(media.ImageDerivativeBackfillItem) error) (media.ImageDerivativeBackfillReport, error) {
		require.True(t, o.AllTenants)
		require.Empty(t, o.TenantUUID)
		for _, item := range plan.Candidates {
			require.Equal(t, item.TenantUUID, o.Expected[item.AssetUUID].TenantUUID)
			item.Outcome, item.JobUUID = "submitted", uuid.NewString()
			if err := emit(item); err != nil {
				return media.ImageDerivativeBackfillReport{}, err
			}
		}
		return media.ImageDerivativeBackfillReport{}, nil
	}}
	var output bytes.Buffer
	require.NoError(t, executeDerivativePlan(context.Background(), stub, plan, p, media.ImageDerivativeBackfillOptions{BatchSize: 2, Concurrency: 1}, json.NewEncoder(&output)))
	require.Contains(t, output.String(), `"scope":"all_tenants"`)
	require.Contains(t, output.String(), otherTenant)
	require.Equal(t, 1, plan.ScanByTenant[otherTenant]["scanned"])
	require.Contains(t, output.String(), `"scan_by_tenant":`)
	err = runDerivativeBackfill([]string{"--plan", global, "--dry-run=false", "--confirm", "--tenant", otherTenant, "--config", "/not-loaded.yaml"})
	require.ErrorContains(t, err, "MEDIA_BACKFILL_PLAN_TENANT_MISMATCH")
	// Removing the v2 tenant owner from a candidate must never be inferred from
	// another record or a default tenant.
	delete(lines[2]["result"].(map[string]any), "tenant_uuid")
	f, err = os.Create(filepath.Join(t.TempDir(), "bad.jsonl"))
	require.NoError(t, err)
	for _, line := range lines {
		require.NoError(t, json.NewEncoder(f).Encode(line))
	}
	require.NoError(t, f.Close())
	_, err = readDerivativePlan(f.Name())
	require.Error(t, err)
}
