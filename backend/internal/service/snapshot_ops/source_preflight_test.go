package snapshot_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPreflightAndCaptureFollowReinstalledPlugin(t *testing.T) {
	s, _, _ := fixtureService(t)
	plugins := t.TempDir()
	install := func(version string) string {
		file := filepath.Join(plugins, version, "plugin.yaml")
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
		require.NoError(t, os.WriteFile(file, []byte("version: "+version), 0600))
		return file
	}
	old := install("0.1.76")
	oldKey := "plugins/ai-craft/0.1.76/plugin.yaml"
	s.cfg.Artifacts[oldKey] = old
	core := s.cfg.Artifacts["core/powerx"]
	s.opts.Sources = func(context.Context) (Config, error) {
		current := s.cfg
		current.Artifacts = map[string]string{"core/powerx": core}
		versions, e := os.ReadDir(plugins)
		if e != nil {
			return current, e
		}
		for _, v := range versions {
			current.Artifacts["plugins/ai-craft/"+v.Name()+"/plugin.yaml"] = filepath.Join(plugins, v.Name(), "plugin.yaml")
		}
		return current, nil
	}
	before, e := s.Overview(rootContext())
	require.NoError(t, e)
	require.True(t, before.Ready, before.Problems)

	// 同一服务进程中卸载旧插件、安装新版，启动时的路径确实已不存在。
	require.NoError(t, os.RemoveAll(filepath.Dir(old)))
	install("0.1.77")
	after, e := s.Overview(rootContext())
	require.NoError(t, e)
	require.True(t, after.Ready, after.Problems)
	require.Equal(t, old, s.cfg.Artifacts[oldKey], "动态检查不应改写共享配置")
	_, e = s.SavePolicy(rootContext(), Policy{Enabled: true, IntervalMinutes: 60, RetentionDays: 1, RetentionCount: 2})
	require.NoError(t, e)
	j := captureReady(t, s)
	raw, e := os.ReadFile(filepath.Join(s.data(j.ID), "programs/plugins/ai-craft/0.1.77/plugin.yaml"))
	require.NoError(t, e)
	require.Equal(t, "version: 0.1.77", string(raw))
	require.NoFileExists(t, filepath.Join(s.data(j.ID), "programs/plugins/ai-craft/0.1.76/plugin.yaml"))
	// 后续自动触发同样不会被旧目录阻断。
	scheduled, e := s.start(context.Background(), uuid.NewString(), "scheduled")
	require.NoError(t, e)
	require.Eventually(t, func() bool {
		got, e := s.Get(rootContext(), scheduled.ID)
		return e == nil && got.Status == "ready"
	}, 5*time.Second, 10*time.Millisecond)
	waitIdle(t, s)
}

func TestPreflightRefreshesSourceFailureAndNeverFallsBackToStaleSources(t *testing.T) {
	s, backend, _ := fixtureService(t)
	available := false
	s.opts.Sources = func(context.Context) (Config, error) {
		if !available {
			return Config{}, errors.New("Web Admin 发布产物缺失，请先完成 npm run build")
		}
		return s.cfg, nil
	}
	for _, state := range []bool{false, true, false} {
		available = state
		out, e := s.Overview(rootContext())
		require.NoError(t, e)
		require.Equal(t, state, out.Ready)
		if state {
			continue
		}
		require.Equal(t, []string{"Web Admin 发布产物缺失，请先完成 npm run build"}, out.Problems)
		_, e = s.Start(rootContext(), uuid.NewString())
		require.ErrorContains(t, e, out.Problems[0])
		_, e = s.SavePolicy(rootContext(), Policy{Enabled: true, IntervalMinutes: 60, RetentionDays: 1, RetentionCount: 2})
		require.ErrorContains(t, e, out.Problems[0])
	}
	require.Zero(t, backend.captures.Load())
}

func TestPreflightKeepsDeploymentErrorsWhenDynamicSourcesAreReady(t *testing.T) {
	s, _, _ := fixtureService(t)
	s.cfg.SourceProblem = "恢复协调器部署合同无效"
	s.opts.Sources = func(context.Context) (Config, error) {
		cfg := s.cfg
		cfg.SourceProblem = ""
		return cfg, nil
	}
	out, e := s.Overview(rootContext())
	require.NoError(t, e)
	require.False(t, out.Ready)
	require.Equal(t, []string{"恢复协调器部署合同无效"}, out.Problems)
}

func TestPreflightIdentifiesMissingComponentWithoutMisleadingBuildAdvice(t *testing.T) {
	s, _, _ := fixtureService(t)
	missing := filepath.Join(t.TempDir(), "missing")
	s.cfg.Artifacts["plugins/ai-craft/0.1.77/backend/bin"] = missing
	out, e := s.Overview(rootContext())
	require.NoError(t, e)
	require.False(t, out.Ready)
	require.Len(t, out.Problems, 1)
	require.Contains(t, out.Problems[0], "plugins/ai-craft/0.1.77/backend/bin")
	require.NotContains(t, out.Problems[0], "构建")
	delete(s.cfg.Artifacts, "plugins/ai-craft/0.1.77/backend/bin")
	s.cfg.Artifacts["core/web-admin"] = missing
	s.cfg.Artifacts["core/powerx"] = missing
	out, e = s.Overview(rootContext())
	require.NoError(t, e)
	require.Len(t, out.Problems, 2)
	require.Contains(t, out.Problems[0], "core/powerx")
	require.Contains(t, out.Problems[1], "core/web-admin")
	require.Contains(t, out.Problems[1], "npm run build")
}
