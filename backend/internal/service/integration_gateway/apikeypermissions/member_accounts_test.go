package apikeypermissions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemberManagementPermissionsHaveDedicatedCoreNamespace(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "config", "platform_capabilities", "iam_member_accounts.yaml"))
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "members.yaml"), raw, 0600))
	t.Setenv(platformCapabilitiesDirEnv, dir)
	rows, err := loadPlatformCapabilityPermissions()
	require.NoError(t, err)
	actions := map[string]bool{}
	for _, row := range rows {
		require.Equal(t, "iam", row.Module)
		require.Equal(t, "platform_capability", row.Source)
		require.False(t, row.AllowAPIKey, "成员管理界面只允许用户 JWT")
		require.NotEqual(t, "user", row.Resource, "不能复用被插件占用的旧 iam.user triple")
		if row.Resource == "member_accounts" {
			actions[row.Action] = true
		}
	}
	require.Equal(t, map[string]bool{"read": true, "write": true, "delete": true}, actions)
}
