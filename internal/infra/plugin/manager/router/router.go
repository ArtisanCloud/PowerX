package router

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

type apiUpstream struct {
	target     *url.URL
	basePath   string
	healthPath string
}

type adminUpstream struct {
	target *url.URL // Nuxt/Nitro upstream，例如 http://127.0.0.1:62368
}

type DynamicRouter struct {
	basePrefix string
	engine     *gin.Engine

	mu        sync.RWMutex
	adminDirs map[string]string
	adminUps  map[string]adminUpstream
	apis      map[string]apiUpstream

	ctxHMACSecret []byte

	gate *authzGate
}

func NewDynamicRouter(basePrefix string, engine *gin.Engine) *DynamicRouter {
	dr := &DynamicRouter{
		basePrefix: basePrefix,
		engine:     engine,
		adminDirs:  make(map[string]string),
		adminUps:   make(map[string]adminUpstream),
		apis:       make(map[string]apiUpstream),
	}

	// 1) 无前缀（标准路径）：/_p/:id/...
	grp := engine.Group(basePrefix)
	grp.GET("/:id/admin/*filepath", dr.serveAdmin)
	grp.HEAD("/:id/admin/*filepath", dr.serveAdmin)
	grp.Any("/:id/api/*filepath", dr.serveAPIProxy)

	// 2) 带一层任意前缀（如 /en/_p/... 或 /__up/_p/...）
	//    解决 i18n 前缀或某些中间层把路径包了一层的问题
	grp2 := engine.Group("/:prefix" + basePrefix)
	grp2.GET("/:id/admin/*filepath", dr.serveAdmin)
	grp2.HEAD("/:id/admin/*filepath", dr.serveAdmin)
	grp2.Any("/:id/api/*filepath", dr.serveAPIProxy)

	return dr
}

func (r *DynamicRouter) MountAdminStatic(id, absDir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adminDirs[id] = absDir
}

func (r *DynamicRouter) MountAdminProxy(id string, upstream *url.URL) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adminUps[id] = adminUpstream{target: upstream}
}

func (r *DynamicRouter) MountAPIProxy(id string, upstream *url.URL, basePath string, healthPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if basePath == "" {
		basePath = "/"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	if healthPath == "" {
		healthPath = "/healthz"
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}
	r.apis[id] = apiUpstream{target: upstream, basePath: basePath, healthPath: healthPath}
}

func (r *DynamicRouter) Unmount(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.adminDirs, id)
	delete(r.adminUps, id)
	delete(r.apis, id)
}

func (r *DynamicRouter) BindAuthorizer(a Authorizer, issuer string, ttl time.Duration) {
	r.gate = newAuthzGate(a, issuer, ttl)
}

func (r *DynamicRouter) InstallPolicy(pluginID string, pol *Policy) {
	if r.gate != nil && pol != nil {
		r.gate.InstallPolicy(pluginID, pol)
	}
}

func (r *DynamicRouter) SetContextHMACSecret(secret []byte) {
	if len(secret) == 0 {
		r.ctxHMACSecret = nil
		return
	}
	dup := make([]byte, len(secret))
	copy(dup, secret)
	r.ctxHMACSecret = dup
}

// ===== Admin（前端）统一入口：优先反代 Nuxt/Nitro，未配置则回静态目录 =====
func (r *DynamicRouter) serveAdmin(c *gin.Context) {
	pluginID := c.Param("id")

	r.mu.RLock()
	up, hasProxy := r.adminUps[pluginID]
	r.mu.RUnlock()

	if hasProxy && up.target != nil {
		proxy := httputil.NewSingleHostReverseProxy(up.target)
		orig := proxy.Director
		proxy.Director = func(req *http.Request) {
			if orig != nil {
				orig(req)
			} else {
				req.URL.Scheme = up.target.Scheme
				req.URL.Host = up.target.Host
			}
			// —— 关键1：去掉可选的“前缀段”（如 /en 或 /__up），只保留从 "/_p/" 开始给 Nuxt
			//    这样 Nuxt 端只需配置 app.baseURL = "/_p/<pluginId>/admin/"，不受 /en 影响
			req.URL.Path = stripLeadingPrefixBeforePlugin(req.URL.Path)
			req.URL.RawPath = req.URL.Path

			// —— 关键2：允许被宿主 iframe 嵌入（覆盖上游安全头）
			req.Host = up.target.Host
			req.Header.Set("X-Forwarded-Host", c.Request.Host)
			req.Header.Set("X-Forwarded-Proto", "http")
			req.Header.Set("X-Forwarded-Prefix", r.basePrefix+"/"+pluginID+"/admin")
		}
		// 覆写响应头，允许在宿主页面 iframe 中展示
		proxy.ModifyResponse = func(resp *http.Response) error {
			// 清理可能阻塞 iframe 的头
			resp.Header.Del("X-Frame-Options")
			resp.Header.Del("Frame-Options")
			resp.Header.Del("Content-Security-Policy")
			// 重置允许同源嵌入
			resp.Header.Set("X-Frame-Options", "SAMEORIGIN")
			resp.Header.Set("Content-Security-Policy", "frame-ancestors 'self'")
			return nil
		}

		proxy.ServeHTTP(c.Writer, c.Request)
		return
	}

	// 未挂反代时，落回静态目录（若没有静态目录，会返回 404）
	r.serveAdminStatic(c)
}

func (r *DynamicRouter) serveAdminStatic(c *gin.Context) {
	pluginID := c.Param("id")
	p := c.Param("filepath")
	if p == "" || p == "/" {
		p = "/index.html"
	}

	r.mu.RLock()
	abs, ok := r.adminDirs[pluginID]
	r.mu.RUnlock()
	if !ok || abs == "" {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	absReq := filepath.Join(abs, filepath.Clean(p))
	if !isSubPath(abs, absReq) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	http.ServeFile(c.Writer, c.Request, absReq)
}

// ===== API 反代（带授权预检 + 短期 Token） =====
func (r *DynamicRouter) serveAPIProxy(c *gin.Context) {
	pluginID := c.Param("id")
	clientPath := c.Param("filepath")
	if clientPath == "" {
		clientPath = "/"
	}

	r.mu.RLock()
	up, ok := r.apis[pluginID]
	r.mu.RUnlock()
	if !ok || up.target == nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "plugin api not mounted"})
		return
	}

	// 认证上下文
	var claims reqctx.CoreXClaims
	if v, ok := c.Get("auth_claims"); ok {
		if cc, ok := v.(reqctx.CoreXClaims); ok {
			claims = cc
		}
	}

	// 预检 + 下发短期 Token
	var pluginToken string
	if r.gate != nil {
		tok, allowed, reason := r.gate.CheckAndMint(c.Request.Context(), pluginID, c.Request.Method, clientPath, claims)
		if !allowed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "access denied at gateway", "reason": reason})
			return
		}
		pluginToken = tok
	}

	proxy := httputil.NewSingleHostReverseProxy(up.target)
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		if origDirector != nil {
			origDirector(req)
		} else {
			req.URL.Scheme = up.target.Scheme
			req.URL.Host = up.target.Host
		}

		// —— 关键：同样兼容 /en/_p/... 的情况，转给下游时去掉前缀段
		origPath := req.URL.Path
		stripped := stripLeadingPrefixBeforePlugin(origPath)

		// 健康检查直通
		clientHealthzWithBase := up.basePath
		if clientHealthzWithBase != "" && clientHealthzWithBase != "/" {
			if !strings.HasSuffix(clientHealthzWithBase, "/") {
				clientHealthzWithBase += "/"
			}
			clientHealthzWithBase += "healthz"
		}
		if clientPath == "/healthz" || (clientHealthzWithBase != "" && clientPath == clientHealthzWithBase) {
			reqPath := joinURLPath(up.target.Path, up.healthPath)
			req.URL.Path = reqPath
			req.URL.RawPath = reqPath
		} else {
			// 拼接下游路径（避免重复 /v1/v1）
			pathForDown := stripped // stripped 仍然包含 "/_p/<id>/api/..." 的前缀，下面做裁剪
			// 截掉 "/_p/<id>/api" 之前的部分，再拼 manifest basePath
			trimmed := trimToAPIClientPath(pathForDown)
			var reqPath string
			if up.basePath != "" && up.basePath != "/" && strings.HasPrefix(trimmed, up.basePath) {
				reqPath = joinURLPath(up.target.Path, trimmed)
			} else {
				reqPath = joinURLPath(up.target.Path, up.basePath, trimmed)
			}
			req.URL.Path = reqPath
			req.URL.RawPath = reqPath
		}

		// 覆盖授权头为插件短期 Token
		req.Header.Del("Authorization")
		if pluginToken != "" {
			req.Header.Set("Authorization", "Bearer "+pluginToken)
		}

		// 透传签名上下文
		if ctxB64, sig, ok := r.buildSignedCtx(c); ok {
			req.Header.Set("X-PowerX-CTX", ctxB64)
			req.Header.Set("X-PowerX-CTX-SIG", sig)
		}

		req.Host = up.target.Host
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func (r *DynamicRouter) buildSignedCtx(c *gin.Context) (ctxB64, sig string, ok bool) {
	if len(r.ctxHMACSecret) == 0 {
		return "", "", false
	}
	claimsAny, exists := c.Get("auth_claims")
	if !exists {
		return "", "", false
	}
	claims := claimsAny.(reqctx.CoreXClaims)

	raw, _ := json.Marshal(claims)
	ctxB64 = base64.StdEncoding.EncodeToString(raw)

	mac := hmac.New(sha256.New, r.ctxHMACSecret)
	mac.Write([]byte(ctxB64))
	sig = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return ctxB64, sig, true
}

func joinURLPath(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" || p == "/" {
			continue
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		out = strings.TrimRight(out, "/") + p
	}
	if out == "" {
		return "/"
	}
	return out
}

func isSubPath(base, child string) bool {
	absBase, _ := filepath.Abs(base)
	absChild, _ := filepath.Abs(child)
	rel, err := filepath.Rel(absBase, absChild)
	return err == nil && !strings.HasPrefix(rel, "..")
}

// ==== 工具：把 "/en/_p/..." 或 "/__up/_p/..." 这样的前缀剥掉，保留从 "/_p/" 开始 ====
// 例如："/en/_p/com.powerx.plugins.base/admin/..." -> "/_p/com.powerx.plugins.base/admin/..."
func stripLeadingPrefixBeforePlugin(p string) string {
	if p == "" || p[0] != '/' {
		return p
	}
	segs := strings.SplitN(p, "/", 4) // ["", maybePrefix, rest...]
	if len(segs) >= 3 && segs[1] != "" && segs[1] != "_p" && segs[2] == "_p" {
		// 形如 "/<prefix>/_p/..."
		if len(segs) >= 4 {
			return "/_p/" + strings.TrimLeft(segs[3], "/")
		}
		return "/_p/"
	}
	// 也兼容 "/__up/_p/..."（某些前端运行时注入的前缀）
	if len(segs) >= 3 && segs[1] == "__up" && segs[2] == "_p" {
		if len(segs) >= 4 {
			return "/_p/" + strings.TrimLeft(segs[3], "/")
		}
		return "/_p/"
	}
	return p
}

// 从完整路径中裁剪出“客户端想要的 API 相对路径”
// 例如："/_p/<id>/api/v1/ping" -> "/v1/ping"
func trimToAPIClientPath(p string) string {
	p = stripLeadingPrefixBeforePlugin(p)
	// 期待形如 "/_p/<id>/api/..."
	if !strings.HasPrefix(p, "/_p/") {
		return p
	}
	rest := strings.TrimPrefix(p, "/_p/")
	i := strings.Index(rest, "/api/")
	if i < 0 {
		return p
	}
	return rest[i+len("/api/")-1:] // 保留前导斜杠
}
