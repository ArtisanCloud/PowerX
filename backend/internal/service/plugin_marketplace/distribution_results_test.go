package plugin_marketplace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	market "github.com/ArtisanCloud/PowerX/internal/infra/plugin/marketplace"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

// 只阻断 t.TempDir 内的交付日志原子写入，保留已落盘文件，允许确定性恢复。
func blockResultJournal(t *testing.T, directory string) func() {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(directory, "*", "distribution.json"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	file := files[0]
	require.NoError(t, os.Rename(file, file+".held"))
	require.NoError(t, os.Mkdir(file, 0o700))
	var once sync.Once
	restore := func() {
		once.Do(func() { require.NoError(t, os.Remove(file)); require.NoError(t, os.Rename(file+".held", file)) })
	}
	t.Cleanup(restore)
	return restore
}
func dueResultWrite(t *testing.T, s *Service, id string) {
	t.Helper()
	d := s.distribution
	d.resultMu.Lock()
	defer d.resultMu.Unlock()
	require.NotNil(t, d.results[id])
	d.results[id].NextRetry = time.Time{}
}
func pendingResultPayloads(s *Service, id string) []market.TransferReport {
	d := s.distribution
	d.resultMu.Lock()
	defer d.resultMu.Unlock()
	var result []market.TransferReport
	for _, event := range d.results[id].Results {
		if event.Report != nil {
			result = append(result, *event.Report)
		}
	}
	return result
}
func resultPlugin(s *Service) plugin_mgr.Plugin {
	p := plugin_mgr.Plugin{ID: "com.test.demo", Version: "1.2.0", Revision: "new-revision", InstallOrigin: s.Origin()}
	p.InstallOrigin.SHA256 = strings.Repeat("a", 64)
	return p
}

func TestDistributionResultWriteFailureRecoversLiveWithoutTouchingOtherInstall(t *testing.T) {
	f := newDistributionFixture(t)
	dir := t.TempDir()
	var inventoryReads atomic.Int32
	s := distributionService(t, f, dir, "instance-a", "pxm_enroll_a", func(context.Context) ([]plugin_mgr.Plugin, error) { inventoryReads.Add(1); return nil, nil })
	require.NoError(t, s.ConnectInstance(context.Background()))
	id, _ := preparedAttempt(t, s, false)
	downloadedAttempt(t, s, id)
	other, _ := preparedAttempt(t, s, false)
	downloadedAttempt(t, s, other)
	require.NoError(t, s.FlushDistribution(context.Background()))
	s.StartDistribution(context.Background())
	// 收到心跳不代表本地日志已写完；等待成功落盘后的状态再注入故障。
	require.Eventually(t, func() bool {
		return !s.distribution.store.Identity().LastHeartbeat.IsZero()
	}, time.Second, 5*time.Millisecond)
	restore := blockResultJournal(t, dir)
	require.Error(t, s.RecordInstalled(id, resultPlugin(s)))
	status := s.Status().Instance
	require.Equal(t, 1, status.UnconfirmedInstalls)
	require.Zero(t, status.PendingReports)
	require.Equal(t, "MARKETPLACE_INSTALL_RESULT_UNCONFIRMED", status.ErrorCode)
	frozen := pendingResultPayloads(s, id)
	require.Len(t, frozen, 1)
	restore()
	require.Eventually(t, func() bool {
		return s.distribution.store.Attempt(id).Phase == "installed" && s.Status().Instance.UnconfirmedInstalls == 0
	}, 8*time.Second, 20*time.Millisecond)
	s.CloseDistribution()
	require.Equal(t, "install", s.distribution.store.Attempt(other).Phase)
	require.False(t, s.distribution.store.Attempt(other).NeedsReview)
	require.False(t, s.distribution.recovering.Load())
	require.Zero(t, inventoryReads.Load())
	got := s.distribution.store.Attempt(id)
	require.Equal(t, frozen[0].EventID, got.Events[1].Payload.EventID)
	require.Equal(t, *frozen[0].ReportedAt, *got.Events[1].Payload.ReportedAt)
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.downloadKeys, 2)
}

func TestDistributionResultWriteRetainsInstalledAndTerminalOutcome(t *testing.T) {
	for _, terminal := range []string{"enabled", "failed", "manual_enabled"} {
		t.Run(terminal, func(t *testing.T) {
			f := newDistributionFixture(t)
			dir := t.TempDir()
			s := distributionService(t, f, dir, "instance-a", "pxm_enroll_a", nil)
			require.NoError(t, s.ConnectInstance(context.Background()))
			id, _ := preparedAttempt(t, s, true)
			downloadedAttempt(t, s, id)
			require.NoError(t, s.FlushDistribution(context.Background()))
			restore := blockResultJournal(t, dir)
			p := resultPlugin(s)
			require.Error(t, s.RecordInstalled(id, p))
			if terminal == "failed" {
				require.Error(t, s.RecordFailed(id, "POWERX_ENABLE_FAILED"))
				require.Error(t, s.RecordFailed(id, "POWERX_ENABLE_FAILED"))
			} else if terminal == "manual_enabled" {
				require.Error(t, s.RecordPluginEnableResult(p, "tenant-a", true))
			} else {
				require.Error(t, s.RecordEnabled(id))
				require.Error(t, s.RecordEnabled(id))
			}
			frozen := pendingResultPayloads(s, id)
			require.Len(t, frozen, 2)
			require.Equal(t, "installed", frozen[0].Status)
			if terminal == "failed" {
				require.Equal(t, "enable", frozen[1].Stage)
			}
			require.Equal(t, 1, s.Status().Instance.UnconfirmedInstalls)
			restore()
			dueResultWrite(t, s, id)
			require.NoError(t, s.FlushDistribution(context.Background()))
			phase := "enabled"
			if terminal == "failed" {
				phase = "failed"
			}
			a := s.distribution.store.Attempt(id)
			require.Equal(t, phase, a.Phase)
			require.Len(t, a.Events, 3)
			require.Zero(t, s.Status().Instance.UnconfirmedInstalls)
			for i, event := range frozen {
				want, err := json.Marshal(event)
				require.NoError(t, err)
				got, err := json.Marshal(a.Events[i+1].Payload)
				require.NoError(t, err)
				require.JSONEq(t, string(want), string(got))
			}
			require.NoError(t, s.FlushDistribution(context.Background()))
			require.Len(t, f.downloadKeys, 1)
			require.Len(t, f.accepted, 3)
		})
	}
}

func TestDistributionFailedResultWriteRecoversEachFailureStage(t *testing.T) {
	for _, stage := range []string{"download", "install", "enable"} {
		t.Run(stage, func(t *testing.T) {
			f := newDistributionFixture(t)
			dir := t.TempDir()
			s := distributionService(t, f, dir, "instance-a", "pxm_enroll_a", nil)
			require.NoError(t, s.ConnectInstance(context.Background()))
			id, _ := preparedAttempt(t, s, false)
			if stage != "download" {
				downloadedAttempt(t, s, id)
			}
			if stage == "enable" {
				installedAttempt(t, s, id)
			}
			require.NoError(t, s.FlushDistribution(context.Background()))
			restore := blockResultJournal(t, dir)
			require.Error(t, s.RecordFailed(id, "POWERX_TEST_FAILURE"))
			require.Equal(t, 1, s.Status().Instance.UnconfirmedInstalls)
			frozen := pendingResultPayloads(s, id)
			require.Equal(t, stage, frozen[0].Stage)
			restore()
			dueResultWrite(t, s, id)
			require.NoError(t, s.FlushDistribution(context.Background()))
			a := s.distribution.store.Attempt(id)
			last := a.Events[len(a.Events)-1].Payload
			require.Equal(t, frozen[0].EventID, last.EventID)
			require.Equal(t, stage, last.Stage)
			require.Equal(t, "failed", a.Phase)
			require.Zero(t, s.Status().Instance.UnconfirmedInstalls)
		})
	}
}

func TestDistributionEmptyTenantCannotConfirmOrFailAnotherTenant(t *testing.T) {
	for _, tenant := range []string{"", " ", "other-tenant", "tenant-a"} {
		for _, success := range []bool{false, true} {
			t.Run(tenant+"/"+map[bool]string{true: "success", false: "failure"}[success], func(t *testing.T) {
				f := newDistributionFixture(t)
				s := distributionService(t, f, t.TempDir(), "instance-a", "pxm_enroll_a", nil)
				require.NoError(t, s.ConnectInstance(context.Background()))
				id, _ := preparedAttempt(t, s, false)
				downloadedAttempt(t, s, id)
				installedAttempt(t, s, id)
				require.NoError(t, s.RecordPluginEnableResult(resultPlugin(s), tenant, success))
				a := s.distribution.store.Attempt(id)
				if tenant == "tenant-a" {
					expected := "failed"
					if success {
						expected = "enabled"
					}
					require.Equal(t, expected, a.Phase)
					require.Len(t, a.Events, 3)
				} else {
					require.Equal(t, "installed", a.Phase)
					require.Len(t, a.Events, 2)
				}
			})
		}
	}
}

func TestDistributionUnconfirmedResultCountDoesNotDoubleCountReview(t *testing.T) {
	f := newDistributionFixture(t)
	dir := t.TempDir()
	s := distributionService(t, f, dir, "instance-a", "pxm_enroll_a", nil)
	require.NoError(t, s.ConnectInstance(context.Background()))
	id, _ := preparedAttempt(t, s, false)
	downloadedAttempt(t, s, id)
	installedAttempt(t, s, id)
	require.NoError(t, s.distribution.store.Update(func(st *market.DistributionState) error { st.Attempts[id].NeedsReview = true; return nil }))
	restore := blockResultJournal(t, dir)
	require.Error(t, s.RecordEnabled(id))
	require.Equal(t, 1, s.Status().Instance.UnconfirmedInstalls)
	restore()
	dueResultWrite(t, s, id)
	require.NoError(t, s.FlushDistribution(context.Background()))
	require.Zero(t, s.Status().Instance.UnconfirmedInstalls)
}

func TestDistributionRestartAfterUnwrittenTerminalResultStaysConservative(t *testing.T) {
	f := newDistributionFixture(t)
	dir := t.TempDir()
	var installed []plugin_mgr.Plugin
	lookup := func(context.Context) ([]plugin_mgr.Plugin, error) { return installed, nil }
	s := distributionService(t, f, dir, "instance-a", "pxm_enroll_a", lookup)
	require.NoError(t, s.ConnectInstance(context.Background()))
	id, _ := preparedAttempt(t, s, true)
	downloadedAttempt(t, s, id)
	installed = []plugin_mgr.Plugin{resultPlugin(s)}
	restore := blockResultJournal(t, dir)
	require.Error(t, s.RecordInstalled(id, installed[0]))
	require.Error(t, s.RecordEnabled(id))
	restore()
	s.CloseDistribution()
	s = distributionService(t, f, dir, "instance-a", "", lookup)
	require.NoError(t, s.RecoverDistribution(context.Background()))
	require.NoError(t, s.FlushDistribution(context.Background()))
	a := s.distribution.store.Attempt(id)
	require.Equal(t, "installed", a.Phase)
	require.True(t, a.NeedsReview)
	require.Equal(t, 1, s.Status().Instance.UnconfirmedInstalls)
	require.Len(t, f.downloadKeys, 1)
}
