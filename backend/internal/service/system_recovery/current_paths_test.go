package system_recovery

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCurrentRestoreTargetAcceptsOnlyVerifiedDarwinSystemAliases(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	file := filepath.Join(root, "not-created", "file")
	physical, err := currentRestoreTarget(file)
	require.NoError(t, err)
	require.Equal(t, file, physical)
	link := filepath.Join(root, "business-link")
	require.NoError(t, os.Symlink(root, link))
	_, err = currentRestoreTarget(filepath.Join(link, "new-file"))
	require.ErrorContains(t, err, "非系统软链接")
	require.ErrorContains(t, err, link)
	if runtime.GOOS == "darwin" {
		for _, alias := range []string{"/var", "/tmp", "/etc"} {
			input := filepath.Join(alias, "powerx-not-created", "file")
			got, e := currentRestoreTarget(input)
			require.NoError(t, e)
			require.Equal(t, "/private"+input, got)
		}
	}
}

func TestCurrentPreparationWithMixedDarwinAliasesInstallsAndRevertsPhysicalTargets(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS system aliases")
	}
	r, cfg, source := currentFixture(t)
	root, err := filepath.EvalSymlinks(filepath.Dir(r.spec.ConfigFile))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(root, "/private/"))
	alias := strings.TrimPrefix(root, "/private")
	r.spec.ConfigFile, r.spec.Executable = filepath.Join(root, "config.yaml"), filepath.Join(root, "powerx")
	source.MediaRoot = filepath.Join(alias, "media")
	cfg.Plugin.InstalledDir = filepath.Join(alias, "plugins")
	cfg.Auth.JWTSecret = token()
	cfg.Server.SecretKey = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("r", 32)))
	t.Setenv("POWERX_AUTH_JWT_SECRET", cfg.Auth.JWTSecret)
	t.Setenv("CORE_X_AUTH_ISSUER", cfg.Auth.Issuer)
	require.NoError(t, os.WriteFile(filepath.Join(root, "media/state"), []byte("B"), 0600))
	require.NoError(t, os.WriteFile(r.spec.Executable, []byte("core-B"), 0700))
	staging, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(staging, "programs/core"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(staging, "programs/core/powerx"), []byte(CurrentRecoveryProtocol+CurrentCacheProtocol), 0700))
	require.NoError(t, os.MkdirAll(filepath.Join(staging, "media"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(staging, "media/state"), []byte("A"), 0600))
	id := uuid.NewString()
	require.NoError(t, privateDirectory(r.work(id)))
	journal, err := r.prepareCurrentFiles(context.Background(), &snapshots.ManagedJob{ID: id}, source, staging, nil, cfg)
	require.NoError(t, err)
	for _, f := range journal.Files {
		require.True(t, canonicalNewPath(f.Target), f.Target)
		require.True(t, strings.HasPrefix(f.Target, root+"/"), f.Target)
	}
	require.NoError(t, r.installCurrentFiles(context.Background(), &journal))
	data, err := os.ReadFile(filepath.Join(root, "media/state"))
	require.NoError(t, err)
	require.Equal(t, "A", string(data))
	require.NoError(t, r.revertCurrentFiles(journal))
	data, err = os.ReadFile(filepath.Join(root, "media/state"))
	require.NoError(t, err)
	require.Equal(t, "B", string(data))
	// 不能借 /var 与 /private/var 的不同写法绕过目标嵌套检查。
	source.Artifacts["core/web-admin"] = filepath.Join(root, "media", "nested-web")
	secondID := uuid.NewString()
	require.NoError(t, privateDirectory(r.work(secondID)))
	_, err = r.prepareCurrentFiles(context.Background(), &snapshots.ManagedJob{ID: secondID}, source, staging, nil, cfg)
	require.ErrorContains(t, err, "相互包含")
	require.ErrorContains(t, err, filepath.Join(root, "media"))
}

// 显式只读审计当前安装的登记路径；不启动业务进程、不准备或替换文件。
func TestCurrentRegisteredRestorePathAudit(t *testing.T) {
	directory := os.Getenv("POWERX_RECOVERY_PATH_AUDIT_DIRECTORY")
	if directory == "" {
		t.Skip("explicit read-only registered path audit")
	}
	for _, name := range []string{"source.enc", "master.key"} {
		info, err := os.Stat(filepath.Join(directory, name))
		require.NoError(t, err)
		require.True(t, info.Mode().IsRegular())
	}
	service := snapshots.NewService(context.Background(), snapshots.Config{Directory: directory}, snapshots.Options{Coordinator: true})
	defer service.Close()
	source, err := service.LoadCoordinatorSource()
	require.NoError(t, err)
	cfg, err := sourceConfig(source)
	require.NoError(t, err)
	paths := map[string]string{"media": source.MediaRoot, "web": source.Artifacts["core/web-admin"], "capabilities": source.Artifacts["core/platform_capabilities"], "plugins": cfg.Plugin.InstalledDir, "registry": cfg.Plugin.RegistryFile}
	for logical, path := range source.ConfigFiles {
		if strings.HasPrefix(logical, "core/files/") {
			paths[logical] = path
		}
	}
	roles := make([]string, 0, len(paths))
	for role := range paths {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	physicalTargets := map[string]string{}
	for _, role := range roles {
		if paths[role] == "" {
			continue
		}
		physical, err := currentRestoreTarget(paths[role])
		require.NoError(t, err, role)
		for other, previousRole := range physicalTargets {
			require.False(t, inside(other, physical) || inside(physical, other), "登记目标重叠：%s 与 %s", previousRole, role)
		}
		require.False(t, inside(directory, physical) || inside(physical, directory), "恢复目标与快照目录重叠：%s", role)
		physicalTargets[physical] = role
		t.Log(fmt.Sprintf("%s: %s -> %s", role, paths[role], physical))
	}
}
