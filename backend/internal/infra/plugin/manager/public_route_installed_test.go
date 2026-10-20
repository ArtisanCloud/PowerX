package manager

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	pmrouter "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
)

// 显式启用的安装环境验证：读取已有 manifest，转发到操作者运行的插件，不启停插件或写 DB。
// POWERX_PUBLIC_GATEWAY_SIGNED_QUERY 仅接收短期有效查询，不在日志中输出渠道凭据。
func TestPublicExposureInstalledPlugin(t *testing.T) {
	root := os.Getenv("POWERX_PUBLIC_GATEWAY_PLUGIN_ROOT")
	upstream := os.Getenv("POWERX_PUBLIC_GATEWAY_UPSTREAM")
	if root == "" || upstream == "" {
		t.Skip("set POWERX_PUBLIC_GATEWAY_PLUGIN_ROOT and POWERX_PUBLIC_GATEWAY_UPSTREAM for installed-plugin acceptance")
	}
	m, err := loadManifestWithCatalogs(root) // 不使用会重写 merged manifest 的 LoadDescriptor。
	if err != nil {
		t.Fatal(err)
	}
	p := plugin_mgr.Plugin{ID: m.ID, Endpoints: m.Endpoints, Routes: m.Routes, RBAC: m.RBAC, Exposure: m.Exposure, Permissions: m.Permissions}
	target, err := url.Parse(upstream)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var authCalls atomic.Int32
	jwtMW := middleware.APIKeyOrJwtMiddleware(nil, []byte("installed-test-secret"), "powerx-auth", []string{"powerx:user"}, []string{"access"}, nil)
	dr := pmrouter.NewDynamicRouter("/_p", engine, func(c *gin.Context) { authCalls.Add(1); jwtMW(c) })
	dr.BindAuthorizer(nil, "powerx-auth", 0)
	InstallPolicy(dr, p.ID, PolicyFromPlugin(p))
	dr.MountAPIProxy(p.ID, target, p.Endpoints.HTTPBasePath, "/healthz")
	host := httptest.NewServer(engine)
	t.Cleanup(host.Close)
	cwd, _ := os.Getwd()
	t.Logf("agent-owned installed acceptance gateway: PID=%d command=go test cwd=%s listener=%s; existing plugin upstream=%s; cleanup=httptest.Server.Close", os.Getpid(), cwd, host.URL, upstream)
	base := strings.TrimRight(p.Endpoints.HTTPBasePath, "/")
	if strings.HasPrefix(base, "/api/") {
		base = strings.TrimPrefix(base, "/api")
	}
	signedQuery := os.Getenv("POWERX_PUBLIC_GATEWAY_SIGNED_QUERY")
	for _, tc := range []struct {
		name, path, signature string
		want                  int
		public                bool
	}{
		{"missing signature", publicProxyStaticPath, "", 401, true},
		{"invalid signature", publicProxyStaticPath, strings.Replace(signedQuery, "signature=", "signature=invalid", 1), 401, true},
		{"valid signature", publicProxyStaticPath, signedQuery, 200, true},
		{"undeclared management API", "/admin/operations/ai-craft/not-declared", signedQuery, 401, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "valid signature" && signedQuery == "" {
				t.Skip("no valid channel query supplied")
			}
			beforeAuth := authCalls.Load()
			req, err := http.NewRequest("POST", host.URL+"/_p/"+p.ID+"/api"+base+tc.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(publicProxyQueryHeader, tc.signature)
			req.Header.Set("X-PowerX-Plugin-Public-Route", "1")
			resp, err := host.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.want {
				t.Fatalf("status=%d want=%d body=%s", resp.StatusCode, tc.want, body)
			}
			if tc.public {
				if authCalls.Load() != beforeAuth || resp.Header.Get("X-PowerX-Upstream-Plugin") != p.ID {
					t.Fatal("public request did not reach installed plugin")
				}
				if tc.want == 401 && !strings.Contains(string(body), "RUNTIME_IDENTITY_UNAUTHORIZED") {
					t.Fatalf("expected installed plugin rejection: %s", body)
				}
				if tc.want == 200 && (!strings.Contains(string(body), m.Version) || !strings.Contains(string(body), "running")) {
					t.Fatalf("unexpected installed runtime identity: %s", body)
				}
			} else if authCalls.Load() != beforeAuth+1 || !strings.Contains(string(body), "IAM_CONTEXT_USER_SESSION_INVALID") {
				t.Fatalf("undeclared API did not require Core user identity: %s", body)
			}
			t.Logf("status=%d reached_installed_plugin=%t core_auth_calls=%d", resp.StatusCode, tc.public, authCalls.Load()-beforeAuth)
		})
	}
}
