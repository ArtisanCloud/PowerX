package manager

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	pmrouter "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	identity "github.com/ArtisanCloud/PowerX/internal/service/plugin_identity"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
)

const (
	publicProxyPluginID     = "com.powerx.plugins.ai-craft"
	publicProxyStaticPath   = "/integration/ai-craft/shopify-storefront/runtime-identity/query"
	publicProxyEventsPath   = "/integration/ai-craft/shopify-storefront/sessions/session-a/events"
	publicProxyQueryHeader  = "X-AI-Craft-Shopify-App-Proxy-Query"
	publicProxySessionToken = "shopify.test.session-token"
	// 固定测试向量：包含重复参数和百分号编码，验证原始查询没有被代理重写。
	publicProxySignedQuery = "extra=b&extra=a&logged_in_customer_id=42&path_prefix=%2Fapps%2Fagent&shop=gateway-test.myshopify.com&timestamp=1760000000&signature=2c53f3d44273df8476257c3eef89787ef56dc4abd16cb24efa0a1b2dea3bd897"
)

func TestPublicExposureProxyFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, base := range []string{"/api/v1", "/api/v2", "/api/custom/v3", "/v3", "/custom-http/v3", "/", "/api/v2/"} {
		t.Run(base, func(t *testing.T) {
			p := publicProxyPlugin(base)
			pol := PolicyFromPlugin(p)
			for _, route := range pol.PublicRoutes {
				if strings.HasPrefix(route.Path, strings.TrimRight(base, "/")+"/integration/") && base != "/" {
					t.Fatalf("public policy still contains HTTP base: %+v", route)
				}
			}

			type forwardedRequest struct {
				path, query, signature, sessionToken, authorization, ticket, body string
			}
			forwarded := make(chan forwardedRequest, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				select {
				case forwarded <- forwardedRequest{r.URL.Path, r.URL.RawQuery, r.Header.Get(publicProxyQueryHeader), r.Header.Get("X-Shopify-Session-Token"), r.Header.Get("Authorization"), r.Header.Get(identity.Header), string(body)}:
				default:
					http.Error(w, "previous unexpected upstream request was not consumed", http.StatusInternalServerError)
					return
				}
				w.Header().Set("X-Test-Channel-Verifier", "shopify_app_proxy")
				if strings.HasSuffix(r.URL.Path, publicProxyStaticPath) && (r.URL.RawQuery != "" || len(body) != 0) {
					http.Error(w, "RUNTIME_IDENTITY_INVALID_ARGUMENT", http.StatusBadRequest)
					return
				}
				if r.Header.Get("X-PowerX-Plugin-Public-Route") != "1" {
					http.Error(w, "missing Core route marker", http.StatusInternalServerError)
					return
				}
				if !verifyPublicProxySignature(r.Header.Get(publicProxyQueryHeader)) {
					http.Error(w, "PLUGIN_SHOPIFY_SIGNATURE_INVALID", http.StatusUnauthorized)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"service":"powerx","environment":"dev","version":"0.1.76","status":"running"}`)
			}))
			t.Cleanup(upstream.Close)
			target, err := url.Parse(upstream.URL + "/upstream-prefix")
			if err != nil {
				t.Fatal(err)
			}

			engine := gin.New()
			var authCalls, guardCalls atomic.Int32
			jwtMW := middleware.APIKeyOrJwtMiddleware(nil, []byte("test-secret"), "powerx-auth", []string{"powerx:user"}, []string{"access"}, nil)
			dr := pmrouter.NewDynamicRouter("/_p", engine, func(c *gin.Context) {
				authCalls.Add(1)
				jwtMW(c)
			})
			dr.BindAuthorizer(nil, "powerx-auth", 0)
			dr.BindTenantPluginGuard(func(context.Context, string, string) error {
				guardCalls.Add(1)
				return errors.New("public channel must resolve its own tenant")
			})
			issuer := &publicProxyIdentityIssuer{}
			dr.BindRequestIdentityIssuer(issuer)
			InstallPolicy(dr, p.ID, pol)
			dr.MountAPIProxy(p.ID, target, p.Endpoints.HTTPBasePath, "/healthz")
			host := httptest.NewServer(engine)
			t.Cleanup(host.Close)
			cwd, _ := os.Getwd()
			t.Logf("agent-owned test servers: PID=%d command=go test cwd=%s gateway=%s upstream=%s; cleanup=httptest.Server.Close", os.Getpid(), cwd, host.URL, upstream.URL)

			clientBase := strings.TrimRight(base, "/")
			if strings.HasPrefix(clientBase, "/api/") {
				clientBase = strings.TrimPrefix(clientBase, "/api")
			}
			cases := []struct {
				name, method, path, signature string
				wantStatus                    int
				wantUpstream                  bool
			}{
				{"static", "POST", clientBase + publicProxyStaticPath, publicProxySignedQuery, 200, true},
				{"brace parameter", "GET", clientBase + publicProxyEventsPath, publicProxySignedQuery, 200, true},
				{"colon parameter", "POST", clientBase + "/integration/ai-craft/shopify-storefront/sessions/session-a/messages/query", publicProxySignedQuery, 200, true},
				{"relative gateway path", "POST", publicProxyStaticPath, publicProxySignedQuery, 200, true},
				{"missing signature reaches plugin", "POST", clientBase + publicProxyStaticPath, "", 401, true},
				{"invalid signature reaches plugin", "POST", clientBase + publicProxyStaticPath, strings.Replace(publicProxySignedQuery, "signature=", "signature=0", 1), 401, true},
				{"wrong method", "GET", clientBase + publicProxyStaticPath, publicProxySignedQuery, 401, false},
				{"HEAD not declared", "HEAD", clientBase + publicProxyEventsPath, publicProxySignedQuery, 401, false},
				{"OPTIONS not declared", "OPTIONS", clientBase + publicProxyStaticPath, publicProxySignedQuery, 401, false},
				{"undeclared management API", "POST", clientBase + "/admin/operations/ai-craft/sessions/query", publicProxySignedQuery, 401, false},
				{"protected exposure", "POST", clientBase + "/integration/ai-craft/protected", publicProxySignedQuery, 401, false},
				{"non-REST exposure", "POST", clientBase + "/integration/ai-craft/tool", publicProxySignedQuery, 401, false},
				{"extra level", "GET", clientBase + publicProxyEventsPath + "/extra", publicProxySignedQuery, 401, false},
				{"empty parameter", "GET", clientBase + "/integration/ai-craft/shopify-storefront/sessions//events", publicProxySignedQuery, 401, false},
				{"dot parameter", "GET", clientBase + "/integration/ai-craft/shopify-storefront/sessions/./events", publicProxySignedQuery, 401, false},
				{"encoded slash", "GET", clientBase + "/integration/ai-craft/shopify-storefront/sessions/session-a%2Fextra/events", publicProxySignedQuery, 401, false},
				{"double encoded slash", "GET", clientBase + "/integration/ai-craft/shopify-storefront/sessions/session-a%252Fextra/events", publicProxySignedQuery, 401, false},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					beforeAuth := authCalls.Load()
					requestURL := host.URL + "/_p/" + p.ID + "/api" + tc.path
					query := ""
					if tc.method == "GET" && tc.wantUpstream {
						query = "cursor=keep%2Braw"
						requestURL += "?" + query
					}
					req, err := http.NewRequest(tc.method, requestURL, nil)
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set(publicProxyQueryHeader, tc.signature)
					req.Header.Set("X-Shopify-Session-Token", publicProxySessionToken)
					req.Header.Set("X-PowerX-Plugin-Public-Route", "1") // 伪造标记不能放行未声明入口。
					req.Header.Set(identity.Header, "untrusted-client-ticket")
					if tc.wantUpstream {
						req.Header.Set("Authorization", "Bearer invalid-user-token")
					}
					resp, err := host.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(resp.Body)
					resp.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					if resp.StatusCode != tc.wantStatus {
						t.Fatalf("status=%d want=%d body=%s", resp.StatusCode, tc.wantStatus, body)
					}
					if tc.wantUpstream {
						if authCalls.Load() != beforeAuth {
							t.Fatal("public request entered Core user authentication")
						}
						if resp.Header.Get("X-Test-Channel-Verifier") == "" {
							t.Fatal("response did not originate from plugin verifier")
						}
						var got forwardedRequest
						select {
						case got = <-forwarded:
						default:
							t.Fatal("plugin upstream did not receive request")
						}
						rel := strings.TrimPrefix(tc.path, clientBase)
						if tc.name == "relative gateway path" {
							rel = tc.path
						}
						wantPath := "/upstream-prefix" + strings.TrimRight(base, "/") + rel
						if got.path != wantPath || got.query != query || got.body != "" {
							t.Fatalf("forwarded request=%+v want path=%s and empty body", got, wantPath)
						}
						if got.signature != tc.signature || got.sessionToken != publicProxySessionToken {
							t.Fatalf("channel credentials changed: %+v", got)
						}
						if got.authorization != "" || got.ticket != "" {
							t.Fatal("Core identity was injected or untrusted ticket was forwarded")
						}
						if tc.wantStatus == 401 && !strings.Contains(string(body), "PLUGIN_SHOPIFY_SIGNATURE_INVALID") {
							t.Fatalf("expected plugin rejection: %s", body)
						}
					} else {
						if authCalls.Load() != beforeAuth+1 {
							t.Fatal("undeclared route bypassed Core user authentication")
						}
						if resp.Header.Get("X-PowerX-Plugin-Public-Route") != "" || resp.Header.Get("X-Test-Channel-Verifier") != "" {
							t.Fatal("undeclared route was treated as public")
						}
						select {
						case got := <-forwarded:
							t.Fatalf("protected request reached upstream: %+v", got)
						default:
						}
						if tc.method != "HEAD" && !strings.Contains(string(body), "IAM_CONTEXT_USER_SESSION_INVALID") {
							t.Fatalf("expected Core rejection: %s", body)
						}
					}
					if guardCalls.Load() != 0 || issuer.calls.Load() != 0 {
						t.Fatal("public channel entered tenant/user identity delegation")
					}
				})
			}
		})
	}
}

func TestPublicExposureRejectsConflictingHTTPBases(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p := publicProxyPlugin("/api/v1")
	p.Routes.BasePath = "/api/v2"
	engine := gin.New()
	var authCalls int
	dr := pmrouter.NewDynamicRouter("/_p", engine, func(c *gin.Context) {
		authCalls++
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	dr.BindAuthorizer(nil, "powerx-auth", 0)
	InstallPolicy(dr, p.ID, PolicyFromPlugin(p))
	dr.MountAPIProxy(p.ID, nil, p.Endpoints.HTTPBasePath, "/healthz")
	for _, version := range []string{"v1", "v2"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/_p/"+p.ID+"/api/"+version+publicProxyStaticPath, nil)
		engine.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Fatalf("conflicting bases permitted request: code=%d", rec.Code)
		}
	}
	if authCalls != 2 {
		t.Fatalf("auth calls=%d want=2", authCalls)
	}
}

func TestPublicExposureRejectsDifferentMountedBase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	p := publicProxyPlugin("/api/v2")
	engine := gin.New()
	dr := pmrouter.NewDynamicRouter("/_p", engine, func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	dr.BindAuthorizer(nil, "powerx-auth", 0)
	InstallPolicy(dr, p.ID, PolicyFromPlugin(p))
	dr.MountAPIProxy(p.ID, nil, "/api/v1", "/healthz")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest("POST", "/_p/"+p.ID+"/api/v1"+publicProxyStaticPath, nil))
	if rec.Code != 401 || rec.Header().Get("X-PowerX-Plugin-Public-Route") != "" {
		t.Fatalf("policy and actual mounted base mismatch was treated as public: %d", rec.Code)
	}
}

func publicProxyPlugin(base string) plugin_mgr.Plugin {
	p := plugin_mgr.Plugin{ID: publicProxyPluginID, Endpoints: plugin_mgr.EndpointSpec{HTTPBasePath: base}}
	if strings.HasPrefix(base, "/api/") {
		p.Routes = &plugin_mgr.RouteSpec{BasePath: base}
	}
	for _, route := range []struct{ method, path, auth, channel string }{
		{"POST", publicProxyStaticPath, "public", "rest"},
		{"GET", "/integration/ai-craft/shopify-storefront/sessions/{sessionID}/events", "public", "rest"},
		{"POST", "/integration/ai-craft/shopify-storefront/sessions/:sessionID/messages/query", "public", "rest"},
		{"POST", "/integration/ai-craft/protected", "jwt", "rest"},
		{"POST", "/integration/ai-craft/tool", "public", "agent_tool"},
	} {
		p.Exposure.Channels = append(p.Exposure.Channels, plugin_mgr.ExposureChannel{
			Type: route.channel, Method: route.method, Auth: route.auth,
			Entrypoint: "${POWERX_PLUGIN_HTTP_BASE:-/api/v1}" + route.path,
			Security:   map[string]any{"verifier": "shopify_app_proxy"},
		})
	}
	return p
}

// 仅模拟插件的 HMAC 边界；真实 Shopify 时间窗、店铺绑定及会话归属另做安装验收。
func verifyPublicProxySignature(rawQuery string) bool {
	values, err := url.ParseQuery(rawQuery)
	if err != nil || values.Get("shop") == "" || values.Get("signature") == "" {
		return false
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if key != "signature" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var canonical strings.Builder
	for _, key := range keys {
		canonical.WriteString(key + "=" + strings.Join(values[key], ","))
	}
	mac := hmac.New(sha256.New, []byte("public-gateway-test-secret"))
	mac.Write([]byte(canonical.String()))
	return hmac.Equal([]byte(values.Get("signature")), []byte(hex.EncodeToString(mac.Sum(nil))))
}

type publicProxyIdentityIssuer struct{ calls atomic.Int32 }

func (i *publicProxyIdentityIssuer) Issue(context.Context, identity.IssueInput) (string, error) {
	i.calls.Add(1)
	return "", errors.New("public channel must not mint PowerX user identity")
}
