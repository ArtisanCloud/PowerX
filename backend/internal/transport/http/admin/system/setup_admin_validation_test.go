package system

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetupCompleteRejectsUnconfirmedAdminBeforeProvisioning(t *testing.T) {
	cases := []struct {
		name   string
		change func(*setupAdminConfig)
		error  string
	}{
		{"username missing", func(a *setupAdminConfig) { a.Username = "" }, "adminUsernameRequired"},
		{"username whitespace", func(a *setupAdminConfig) { a.Username = " \t" }, "adminUsernameRequired"},
		{"email missing", func(a *setupAdminConfig) { a.Email = "" }, "adminEmailInvalid"},
		{"email invalid", func(a *setupAdminConfig) { a.Email = "not-an-email" }, "adminEmailInvalid"},
		{"email display name", func(a *setupAdminConfig) { a.Email = "Admin <admin@example.test>" }, "adminEmailInvalid"},
		{"password missing", func(a *setupAdminConfig) { a.Password = "" }, "adminPasswordRequired"},
		{"password whitespace", func(a *setupAdminConfig) { a.Password = " \t" }, "adminPasswordRequired"},
		{"password exceeds bcrypt limit", func(a *setupAdminConfig) { a.Password = strings.Repeat("密", 25) }, "adminPasswordTooLong"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, payload, path := setupJWTFixture(t)
			tc.change(&payload.Admin)
			// 保存草稿仍允许尚未确认管理员的输入。
			require.NoError(t, validateSetupDraftConfig(payload))
			require.NoError(t, h.storeDraftConfig(payload))
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/setup/complete", nil)
			h.Complete(c)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "setup.errors."+tc.error)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after, "拒绝请求不得写运行配置、密钥或执行数据库子命令")
			require.Equal(t, "uninstalled", config.GlobalConfig.Install.Status)
		})
	}
}

func TestSetupCompleteAcceptsCustomAdmin(t *testing.T) {
	_, payload, _ := setupJWTFixture(t)
	payload.Admin.Username = "custom-admin"
	payload.Admin.Password = strings.Repeat("密", 24) // bcrypt 的 72 字节边界。
	require.NoError(t, validateSetupConfig(payload))
}
