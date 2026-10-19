package system_recovery

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecoveryPortalRootSessionResolvesAPIPrefix(t *testing.T) {
	for _, tc := range []struct {
		name, yamlPrefix, expectedPrefix string
	}{
		{"omitted", "", "/api"},
		{"empty", "    api_prefix: \"\"\n", "/api"},
		{"whitespace", "    api_prefix: \"   \"\n", "/api"},
		{"custom", "    api_prefix: /custom/v3\n", "/custom/v3"},
		{"custom-without-leading-slash", "    api_prefix: custom/v3\n", "/custom/v3"},
		{"custom-trailing-slash", "    api_prefix: /custom/v3/\n", "/custom/v3"},
		{"root", "    api_prefix: /\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			paths := make(chan string, 4)
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths <- r.URL.Path
				if r.URL.Path != tc.expectedPrefix+"/admin/user/auth/me/context" || r.Header.Get("Authorization") != "Bearer prefix-fixture-root" {
					w.WriteHeader(404)
					return
				}
				_, _ = w.Write([]byte(`{"data":{"is_root":true,"user":{"id":7,"is_root":true}}}`))
			}))
			defer core.Close()
			u, err := url.Parse(core.URL)
			require.NoError(t, err)
			port, err := strconv.Atoi(u.Port())
			require.NoError(t, err)
			t.Logf("owned httptest PID=%d command=go test cwd=backend listener=%s; deferred Close releases listener", os.Getpid(), core.URL)
			c, _ := portalFixture(t)
			c.Initial.SourceManaged = true
			require.NoError(t, os.MkdirAll(filepath.Dir(c.Initial.ConfigFile), 0700))
			require.NoError(t, os.WriteFile(c.Initial.ConfigFile, []byte(fmt.Sprintf("server:\n    port: %d\n%s", port, tc.yamlPrefix)), 0600))
			require.NoError(t, c.saveActive(Active{Generation: 1, Deployment: c.Initial}))
			r := httptest.NewRequest("POST", recoveryEntry+"session", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer prefix-fixture-root")
			w := httptest.NewRecorder()
			c.gateway(w, r)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Equal(t, tc.expectedPrefix+"/admin/user/auth/me/context", <-paths)
			require.Contains(t, w.Body.String(), `"authenticated":true`)
			require.Len(t, w.Result().Cookies(), 1)
			core.Close()
			require.Equal(t, 200, portalCall(c, "GET", recoveryEntry+"session", nil, w.Result().Cookies()[0], "").Code, "独立会话在业务身份接口停止后仍可使用")
		})
	}
}

func TestRecoveryPortalDefaultPrefixStillRejectsNonRoot(t *testing.T) {
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/admin/user/auth/me/context" {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"is_root":false,"user":{"id":7,"is_root":false}}}`))
	}))
	defer core.Close()
	u, err := url.Parse(core.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	t.Logf("owned httptest PID=%d command=go test cwd=backend listener=%s; deferred Close releases listener", os.Getpid(), core.URL)
	c, _ := portalFixture(t)
	c.Initial.SourceManaged = true
	require.NoError(t, os.MkdirAll(filepath.Dir(c.Initial.ConfigFile), 0700))
	require.NoError(t, os.WriteFile(c.Initial.ConfigFile, []byte(fmt.Sprintf("server:\n    port: %d\n", port)), 0600))
	require.NoError(t, c.saveActive(Active{Generation: 1, Deployment: c.Initial}))
	r := httptest.NewRequest("POST", recoveryEntry+"session", strings.NewReader(`{}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer prefix-fixture-member")
	w := httptest.NewRecorder()
	c.gateway(w, r)
	require.Equal(t, 403, w.Code)
	require.Empty(t, w.Result().Cookies())
}
