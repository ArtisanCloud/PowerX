package router

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/security"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type apiUpstream struct {
	target     *url.URL
	basePath   string
	healthPath string
}

type adminUpstream struct {
	target *url.URL // Nuxt/Nitro upstream，例如 http://127.0.0.1:5xxxx
}

type DynamicRouter struct {
	basePrefix    string // 固定 "/_p"
	engine        *gin.Engine
	mu            sync.RWMutex
	adminDirs     map[string]string
	adminUps      map[string]adminUpstream
	apis          map[string]apiUpstream
	ctxHMACSecret []byte

	gate *authzGate
}

// ===== 构造 & 路由注册 =====

func NewDynamicRouter(basePrefix string, engine *gin.Engine, apiMiddleware ...gin.HandlerFunc) *DynamicRouter {
	dr := &DynamicRouter{
		basePrefix: basePrefix,
		engine:     engine,
		adminDirs:  make(map[string]string),
		adminUps:   make(map[string]adminUpstream),
		apis:       make(map[string]apiUpstream),
	}

	// ---------- 一次性规范化入口：把 /__up/_p/... 302/307 到干净的 /_p/... ----------
	// 1) /__up/_p/:id/admin/*rest → 302 /_p/:id/admin/<rest>   （去掉任何嵌套前缀）
	//engine.Any("/__up/_p/:id/admin/*rest", func(c *gin.Context) {
	//	id := c.Param("id")
	//	rest := strings.TrimLeft(c.Param("rest"), "/")
	//
	//	// 🔒 关键清洗：把多出来的 "__up/_p/<id>/admin/" 或 "_p/<id>/admin/" 前缀剥掉
	//	bad1 := "__up/_p/" + id + "/admin/"
	//	bad2 := "_p/" + id + "/admin/"
	//	if strings.HasPrefix(rest, bad1) {
	//		rest = strings.TrimPrefix(rest, bad1)
	//	} else if strings.HasPrefix(rest, bad2) {
	//		rest = strings.TrimPrefix(rest, bad2)
	//	}
	//
	//	to := "/_p/" + id + "/admin/"
	//	if rest != "" {
	//		to += rest
	//	}
	//	if q := c.Request.URL.RawQuery; q != "" {
	//		to += "?" + q
	//	}
	//	c.Redirect(http.StatusFound, to)
	//})
	//
	//// 2) /__up/_p/:id/api/*rest → 307 /_p/:id/api/<rest>   （同样清洗）
	//engine.Any("/__up/_p/:id/api/*rest", func(c *gin.Context) {
	//	id := c.Param("id")
	//	rest := strings.TrimLeft(c.Param("rest"), "/")
	//
	//	bad1 := "__up/_p/" + id + "/api/"
	//	bad2 := "_p/" + id + "/api/"
	//	if strings.HasPrefix(rest, bad1) {
	//		rest = strings.TrimPrefix(rest, bad1)
	//	} else if strings.HasPrefix(rest, bad2) {
	//		rest = strings.TrimPrefix(rest, bad2)
	//	}
	//
	//	to := "/_p/" + id + "/api/"
	//	if rest != "" {
	//		to += rest
	//	}
	//	if q := c.Request.URL.RawQuery; q != "" {
	//		to += "?" + q
	//	}
	//	c.Redirect(http.StatusTemporaryRedirect, to)
	//})

	// ---------- 标准对外路由：仅这两条 ----------
	grp := engine.Group(basePrefix) // "/_p"
	{
		grp.GET("/:id/admin/*filepath", dr.serveAdmin)
		grp.HEAD("/:id/admin/*filepath", dr.serveAdmin)
	}

	apiGrp := grp.Group("/:id/api")
	if len(apiMiddleware) > 0 {
		apiGrp.Use(apiMiddleware...)
	}
	apiGrp.Any("/*filepath", dr.serveAPIProxy)

	// ---------- 调试端点 ----------
	engine.GET("/__debug/plugins", func(c *gin.Context) {
		dr.mu.RLock()
		defer dr.mu.RUnlock()
		type up struct{ Target string }
		admin := map[string]up{}
		for k, v := range dr.adminUps {
			target := ""
			if v.target != nil {
				target = v.target.String()
			}
			admin[k] = up{Target: target}
		}
		api := map[string]map[string]string{}
		for k, v := range dr.apis {
			target := ""
			if v.target != nil {
				target = v.target.String()
			}
			api[k] = map[string]string{
				"target":     target,
				"basePath":   v.basePath,
				"healthPath": v.healthPath,
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"basePrefix": dr.basePrefix,
			"adminUps":   admin,
			"apis":       api,
		})
	})

	engine.GET("/__debug/policy/:id", func(c *gin.Context) {
		id := c.Param("id")
		if dr.gate == nil {
			c.JSON(200, gin.H{"error": "gate nil"})
			return
		}
		pol := dr.gate.policies[id] // 根据你的 gate 结构取
		if pol == nil {
			c.JSON(200, gin.H{"plugin": id, "policy": "nil"})
			return
		}
		// 列出关键信息（避免把密钥类东西暴露）
		routes := make([]string, 0, len(pol.Routes))
		for k := range pol.Routes {
			routes = append(routes, k)
		}
		res := map[string][]string{}
		for rn, acts := range pol.Resources {
			lst := []string{}
			for a := range acts {
				lst = append(lst, a)
			}
			res[rn] = lst
		}
		c.JSON(200, gin.H{
			"plugin":    id,
			"http_base": pol.HTTPBase,
			"routes":    routes,
			"resources": res,
		})
	})

	return dr
}

// ===== 对外挂载方法 =====

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
	clientPath := c.Param("filepath") // 例如：/、/en/dashboard、/assets/...
	if clientPath == "" {
		clientPath = "/"
	}
	if clean, changed := normalizeAdminClientPath(pluginID, clientPath); changed {
		logger.InfoF(c.Request.Context(), "[ADMIN-CLEAN] plugin=%s raw=%q clean=%q", pluginID, clientPath, clean)
		clientPath = clean
	}

	// 反代优先
	r.mu.RLock()
	up, hasProxy := r.adminUps[pluginID]
	r.mu.RUnlock()

	if hasProxy && up.target != nil {
		proxy := httputil.NewSingleHostReverseProxy(up.target)
		origDirector := proxy.Director

		proxy.Director = func(req *http.Request) {
			if origDirector != nil {
				origDirector(req)
			} else {
				req.URL.Scheme = up.target.Scheme
				req.URL.Host = up.target.Host
			}
			upstreamPath := clientPath
			if shouldRewriteAdminDocToIndex(c.Request, clientPath) {
				upstreamPath = "/"
			}
			// —— 只做传球：/_p/<id>/admin/*filepath（不做任何 locale 注入/剥离）
			req.URL.Path = joinURLPath(r.basePrefix, pluginID, "admin", upstreamPath)
			req.URL.RawPath = req.URL.Path

			// 仅传递宿主挂载点（不含 locale）
			req.Host = up.target.Host
			req.Header.Set("X-Forwarded-Host", c.Request.Host)
			req.Header.Set("X-Forwarded-Proto", "http")
			req.Header.Set("X-Forwarded-Prefix", joinURLPath(r.basePrefix, pluginID, "admin")) // "/_p/<id>/admin"

			// 诊断
			req.Header.Set("X-PX-Upstream", up.target.String())
			req.Header.Set("X-PX-Client-Path", clientPath)
			req.Header.Set("X-PX-Upstream-Path", upstreamPath)
			attachTraceHeaders(c, req)
			req.Header.Set("X-PowerX-Plugin-Id", pluginID)
		}

		proxy.ModifyResponse = func(resp *http.Response) error {
			applyAdminFrameHeaders(resp.Header)
			if resp.StatusCode >= 300 && resp.StatusCode < 400 {
				fixAdminLocationNoLocale(resp) // ✅ 仅做 __up → /_p 的规整
			}
			return nil
		}

		proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			upHost := up.target.String()
			rw.Header().Set("X-PX-Proxy-Error", err.Error())
			rw.Header().Set("X-PX-Proxy-Upstream", upHost)
			c.Error(err)
			rw.WriteHeader(http.StatusBadGateway)
			_, _ = rw.Write([]byte("Bad Gateway (admin)\nupstream=" + upHost + "\nerror=" + err.Error() + "\n"))
		}

		proxy.ServeHTTP(c.Writer, c.Request)
		return
	}

	// 未挂反代时，落回静态目录（若没有，将 404）
	r.serveAdminStatic(c, pluginID, clientPath)
}

func (r *DynamicRouter) serveAdminStatic(c *gin.Context, pluginID, clientPath string) {
	p := clientPath
	if p == "" || p == "/" {
		p = "/index.html"
	} else if clean, changed := normalizeAdminClientPath(pluginID, p); changed {
		logger.InfoF(c.Request.Context(), "[ADMIN-CLEAN-STATIC] plugin=%s raw=%q clean=%q", pluginID, p, clean)
		p = clean
	}
	if shouldRewriteAdminDocToIndex(c.Request, p) {
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

	applyAdminFrameHeaders(c.Writer.Header())
	http.ServeFile(c.Writer, c.Request, absReq)
}

func shouldRewriteAdminDocToIndex(req *http.Request, clientPath string) bool {
	if req == nil {
		return false
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	p := strings.TrimSpace(clientPath)
	if p == "" || p == "/" {
		return false
	}
	lower := strings.ToLower(p)
	if strings.HasPrefix(lower, "/assets/") ||
		strings.HasPrefix(lower, "/_nuxt/") ||
		strings.HasPrefix(lower, "/images/") ||
		strings.HasPrefix(lower, "/favicon") ||
		strings.HasPrefix(lower, "/__") {
		return false
	}
	if ext := strings.ToLower(filepath.Ext(lower)); ext != "" {
		return false
	}
	accept := strings.ToLower(strings.TrimSpace(req.Header.Get("Accept")))
	if accept != "" && !strings.Contains(accept, "text/html") && !strings.Contains(accept, "*/*") {
		return false
	}
	return true
}

// ===== API 反代（带授权预检 + 短期 Token） =====

func (r *DynamicRouter) serveAPIProxy(c *gin.Context) {
	pluginID := c.Param("id")
	clientPath := c.Param("filepath")
	requestID, traceID := getRequestTraceIDs(c)
	logTenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if clientPath == "" {
		clientPath = "/"
	}
	if isIdentityAuthClientPath(clientPath) {
		hostPath := toHostIdentityAuthPath(clientPath)
		if q := c.Request.URL.RawQuery; q != "" {
			hostPath += "?" + q
		}
		logger.InfoF(c.Request.Context(), "[API-REDIRECT] plugin=%s method=%s clientPath=%s tenant_uuid=%s request_id=%s trace_id=%s -> hostPath=%s",
			pluginID, c.Request.Method, clientPath, logTenantUUID, requestID, traceID, hostPath)
		c.Redirect(http.StatusTemporaryRedirect, hostPath)
		c.Abort()
		return
	}
	if r.redirectAdminFromAPI(c, pluginID, clientPath) {
		return
	}
	// === 关键日志：API 入口 ===
	logger.InfoF(c.Request.Context(), "[API-IN] %s %s plugin=%s clientPath=%s tenant_uuid=%s request_id=%s trace_id=%s",
		c.Request.Method, c.Request.URL.Path, pluginID, clientPath, logTenantUUID, requestID, traceID)
	r.mu.RLock()
	up, ok := r.apis[pluginID]
	registered := make([]string, 0, len(r.apis))
	for id := range r.apis {
		registered = append(registered, id)
	}
	r.mu.RUnlock()
	if !ok || up.target == nil {
		logger.InfoF(c.Request.Context(), "[API-MISS] plugin=%s tenant_uuid=%s request_id=%s trace_id=%s registered=%v", pluginID, logTenantUUID, requestID, traceID, registered)
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"error":      "plugin api upstream unavailable",
			"reason":     "plugin not mounted into /_p/{id}/api proxy",
			"plugin_id":  pluginID,
			"registered": registered,
		})
		return
	}

	// 认证上下文（可选）
	var claims reqctx.CoreXClaims
	if v, ok := c.Get("auth_claims"); ok {
		if cc, ok := v.(reqctx.CoreXClaims); ok {
			claims = cc
		}
	}
	// 兜底：部分链路不会把 auth_claims 以 CoreXClaims 直接放入 gin context。
	// 这里从 reqctx 回填，避免插件拿到“空身份/零租户身份”导致 401/403。
	ctxTenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if ctxTenant != "" {
		if canonical, err := reqctx.CanonicalTenantUUID(ctxTenant); err == nil && !isZeroTenantUUID(canonical) {
			claims.TenantUUID = canonical
		}
	}
	if claims.TenantUUID == "" || isZeroTenantUUID(claims.TenantUUID) {
		if rc := reqctx.GetClaims(c.Request.Context()); rc != nil {
			if canonical, err := reqctx.CanonicalTenantUUID(strings.TrimSpace(rc.TenantUUID)); err == nil && !isZeroTenantUUID(canonical) {
				claims.TenantUUID = canonical
			}
		}
	}
	if claims.UserID == 0 {
		claims.UserID = reqctx.GetUserID(c.Request.Context())
	}
	if claims.MemberID == 0 {
		claims.MemberID = reqctx.GetMemberID(c.Request.Context())
	}
	if claims.MemberUUID == "" {
		claims.MemberUUID = strings.TrimSpace(reqctx.GetSubject(c.Request.Context()))
	}
	if rc := reqctx.GetClaims(c.Request.Context()); rc != nil {
		if claims.UserUUID == "" {
			claims.UserUUID = strings.TrimSpace(rc.UserUUID)
		}
	}

	// 预检 + 下发短期 Token（可选）
	var pluginToken string
	gatePath := normalizeGatePathForPolicy(clientPath, up.basePath)
	logger.DebugF(c.Request.Context(), "[GATE-PATH] plugin=%s method=%s raw=%s normalized=%s basePath=%s tenant_uuid=%s request_id=%s trace_id=%s",
		pluginID, c.Request.Method, clientPath, gatePath, up.basePath, logTenantUUID, requestID, traceID)
	if isTenantScopedWSBusPath(gatePath) {
		resolvedTenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
		if resolvedTenant == "" {
			resolvedTenant = strings.TrimSpace(claims.TenantUUID)
		}
		if resolvedTenant == "" {
			logger.WarnF(c.Request.Context(), "[PROXY-TENANT-DENY] plugin=%s method=%s clientPath=%s normalized=%s tenant_uuid=%s request_id=%s trace_id=%s reason=%s",
				pluginID, c.Request.Method, clientPath, gatePath, resolvedTenant, requestID, traceID, "tenant scoped ws-bus route requires tenant from auth context")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":  "access denied at gateway",
				"reason": "tenant scoped ws-bus route requires tenant from auth context",
			})
			return
		}
	}
	if r.gate != nil {
		tok, allowed, reason := r.gate.CheckAndMint(c.Request.Context(), pluginID, c.Request.Method, gatePath, claims)
		if !allowed {
			logger.WarnF(c.Request.Context(), "[GATE-DENY] plugin=%s method=%s clientPath=%s tenant_uuid=%s request_id=%s trace_id=%s reason=%s",
				pluginID, c.Request.Method, gatePath, logTenantUUID, requestID, traceID, reason)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "access denied at gateway", "reason": reason})
			return
		}
		pluginToken = tok
		logger.DebugF(c.Request.Context(), "[GATE-ALLOW] plugin=%s method=%s clientPath=%s tenant_uuid=%s request_id=%s trace_id=%s", pluginID, c.Request.Method, clientPath, logTenantUUID, requestID, traceID)

	}

	proxy := httputil.NewSingleHostReverseProxy(up.target)
	proxy.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Set("X-PowerX-Upstream-Plugin", pluginID)
		resp.Header.Set("X-PowerX-Upstream-Status", http.StatusText(resp.StatusCode))
		if resp.StatusCode < http.StatusBadRequest {
			upstreamRequestID := extractUpstreamRequestID(resp.Header, nil)
			logger.InfoF(c.Request.Context(), "[PROXY-RESP] plugin=%s method=%s req=%s tenant_uuid=%s request_id=%s trace_id=%s upstream_request_id=%s upstream_status=%d",
				pluginID, c.Request.Method, c.Request.URL.Path, logTenantUUID, requestID, traceID, upstreamRequestID, resp.StatusCode)
			return nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			upstreamRequestID := strings.TrimSpace(resp.Header.Get("X-Request-ID"))
			logger.InfoF(c.Request.Context(), "[PROXY-BACKEND-ERR] plugin=%s tenant_uuid=%s request_id=%s trace_id=%s upstream_request_id=%s status=%d read_body_err=%v",
				pluginID, logTenantUUID, requestID, traceID, upstreamRequestID, resp.StatusCode, err)
			return nil
		}
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewBuffer(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))

		msg := string(body)
		if len(msg) > 1024 {
			msg = msg[:1024] + "...(truncated)"
		}
		upstreamRequestID := extractUpstreamRequestID(resp.Header, body)
		logger.InfoF(c.Request.Context(), "[PROXY-BACKEND-ERR] plugin=%s method=%s req=%s tenant_uuid=%s request_id=%s trace_id=%s upstream_request_id=%s upstream_status=%d upstream_body=%q",
			pluginID, c.Request.Method, c.Request.URL.Path, logTenantUUID, requestID, traceID, upstreamRequestID, resp.StatusCode, msg)
		logger.InfoF(c.Request.Context(), "[PROXY-RESP] plugin=%s method=%s req=%s tenant_uuid=%s request_id=%s trace_id=%s upstream_request_id=%s upstream_status=%d",
			pluginID, c.Request.Method, c.Request.URL.Path, logTenantUUID, requestID, traceID, upstreamRequestID, resp.StatusCode)
		return nil
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		logger.InfoF(c.Request.Context(), "[PROXY-TRANSPORT-ERR] plugin=%s method=%s req=%s tenant_uuid=%s request_id=%s trace_id=%s err=%v",
			pluginID, c.Request.Method, c.Request.URL.Path, logTenantUUID, requestID, traceID, err)
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte(`{"error":"plugin upstream transport error"}`))
	}
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		if origDirector != nil {
			origDirector(req)
		} else {
			req.URL.Scheme = up.target.Scheme
			req.URL.Host = up.target.Host
		}

		// 只做传球：把 "/_p/<id>/api/*" 裁剪并归一为策略坐标，再映射到上游路径。
		trimmed := trimToAPIClientPath(joinURLPath(r.basePrefix, pluginID, "api", clientPath))
		normalized := normalizeGatePathForPolicy(trimmed, up.basePath)
		reqPath := buildAPIUpstreamPath(up.target.Path, up.basePath, normalized)
		req.URL.Path = reqPath
		req.URL.RawPath = reqPath
		// === 关键日志：最终上游路径 ===
		logger.DebugF(c.Request.Context(), "[PROXY-OUT] plugin=%s basePath=%s + clientPath=%s tenant_uuid=%s request_id=%s trace_id=%s => upstream=%s",
			pluginID, up.basePath, clientPath, logTenantUUID, requestID, traceID, reqPath)
		// 覆盖授权头为插件短期 Token
		req.Header.Del("Authorization")
		if pluginToken != "" {
			logger.DebugF(c.Request.Context(), "[GATE-TOKEN] plugin=%s tenant_uuid=%s request_id=%s trace_id=%s tid=%s",
				pluginID, logTenantUUID, requestID, traceID, extractJWTStringClaim(pluginToken, "tid"))
			req.Header.Set("Authorization", "Bearer "+pluginToken)
		}
		tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
		if tenantUUID == "" {
			tenantUUID = strings.TrimSpace(claims.TenantUUID)
		}
		if tenantUUID != "" {
			logger.DebugF(c.Request.Context(), "[PROXY-CTX] plugin=%s request_id=%s trace_id=%s tenantUUID=%s", pluginID, requestID, traceID, tenantUUID)
			req.Header.Set("tenant_uuid", tenantUUID)
			req.Header.Set("X-PowerX-Tenant", tenantUUID)
		} else {
			logger.DebugF(c.Request.Context(), "[PROXY-CTX] plugin=%s request_id=%s trace_id=%s tenantUUID missing", pluginID, requestID, traceID)
			req.Header.Del("tenant_uuid")
			req.Header.Del("X-PowerX-Tenant")
		}

		// 透传签名上下文
		if ctxB64, sig, ok := r.buildSignedCtx(c); ok {
			req.Header.Set("X-PowerX-CTX", ctxB64)
			req.Header.Set("X-PowerX-CTX-SIG", sig)
		}
		attachTraceHeaders(c, req)
		req.Header.Set("X-PowerX-Plugin-Id", pluginID)

		req.Host = up.target.Host
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func normalizeGatePathForPolicy(clientPath, basePath string) string {
	path := strings.TrimSpace(clientPath)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	base := strings.TrimSpace(basePath)
	if base == "" {
		return path
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	base = strings.TrimRight(base, "/")

	// 常见场景：route 是 "/_p/:id/api/*filepath"，传入 clientPath 变为 "/v1/..."
	// 而策略希望在 "/api/v1/..." 坐标匹配。这里按插件声明的 basePath 进行归一。
	if strings.HasPrefix(base, "/api/") && strings.HasPrefix(path, "/v1/") {
		return "/api" + path
	}
	if strings.HasPrefix(base, "/api") && !strings.HasPrefix(path, "/api/") {
		return joinURLPath("/api", path)
	}
	return path
}

func buildAPIUpstreamPath(targetPath, basePath, normalizedPath string) string {
	base := strings.TrimSpace(basePath)
	if base == "" {
		base = "/"
	}
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	base = strings.TrimRight(base, "/")
	if base == "" {
		base = "/"
	}

	p := strings.TrimSpace(normalizedPath)
	if p == "" {
		p = "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}

	if base == "/" {
		return joinURLPath(targetPath, p)
	}
	if p == base || strings.HasPrefix(p, base+"/") {
		return joinURLPath(targetPath, p)
	}
	return joinURLPath(targetPath, base, p)
}

func extractJWTStringClaim(token, claim string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if v, ok := payload[claim].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func isRootTenantUUID(v string) bool {
	normalized := strings.TrimSpace(strings.ToLower(v))
	return normalized == "00000000-0000-0000-0000-000000000000" ||
		normalized == "00000000-0000-0000-0000-000000000001"
}

func isZeroTenantUUID(v string) bool {
	return isRootTenantUUID(v)
}

func isTenantScopedWSBusPath(path string) bool {
	p := strings.TrimSpace(path)
	return strings.HasSuffix(p, "/admin/runtime/ws-bus/test-flow") ||
		strings.HasSuffix(p, "/admin/runtime/ws-bus/grant") ||
		strings.HasSuffix(p, "/admin/runtime/ws-bus/publish")
}

// ===== 工具/辅助 =====

func applyAdminFrameHeaders(h http.Header) {
	h.Del("X-Frame-Options")
	h.Del("Frame-Options")
	h.Del("Content-Security-Policy")
	h.Del("Content-Security-Policy-Report-Only")
	h.Set("Content-Security-Policy", security.BuildFrameAncestorsCSP())
	h.Set("X-Frame-Options", "SAMEORIGIN")
}

// 清洗 3xx Location：
//  1. "__up/_p" 前缀 -> "/_p"
//  2. 将 "/:locale/_p/<id>/admin/..." 规整为 "/_p/<id>/admin/:locale/..."
//     —— 幂等：若尾部已是同一个 :locale 开头，不再重复插入
//  3. 压平重复 "/_p/<id>/admin/_p/<id>/admin" 段
func fixBadAdminLocation(resp *http.Response, pluginID string) {
	if resp.StatusCode < http.StatusMultipleChoices || resp.StatusCode >= http.StatusBadRequest {
		return
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return
	}
	u, err := url.Parse(loc)
	if err != nil {
		return
	}
	orig := u.Path
	if orig == "" {
		return
	}

	clean := orig

	// (1) "__up/_p" -> "/_p"
	clean = strings.ReplaceAll(clean, "/__up/_p/", "/_p/")

	// (2) 处理 "/:locale/_p/<id>/admin/..." 形态
	// 仅当第一段是疑似 locale，第二段是 "_p" 时才尝试
	if strings.HasPrefix(clean, "/") {
		// 去掉最前导 "/"，以便分段
		head := strings.TrimLeft(clean, "/")
		parts := strings.SplitN(head, "/", 3) // [locale, "_p", rest]
		if len(parts) >= 2 && parts[1] == "_p" {
			locale := parts[0]
			if looksLikeLocaleSegment(locale) && locale != "__up" {
				rest := ""
				if len(parts) == 3 {
					rest = parts[2] // "<id>/admin/xxxx..."
				}
				// 仅当 rest 真正是 "<id>/admin..." 才重写，防误伤
				prefix := pluginID + "/admin"
				if pluginID != "" && strings.HasPrefix(rest, prefix) {
					// 取出 admin 之后的 tail，比如 "/en/dashboard" 或 "/dashboard" 或 ""
					tail := strings.TrimPrefix(rest, prefix)
					if tail == "" {
						tail = "/"
					}
					// —— 幂等关键：如果 tail 已经以相同 locale 起始，就不要再插入一次
					// 允许 "/en" 或 "/en/..." 两种情况
					if strings.HasPrefix(tail, "/"+locale) &&
						(len(tail) == 1+len(locale) || tail[1+len(locale)] == '/') {
						// 已包含 locale：只把 locale 从头部移到尾部（实际上尾部已有，无需变更）
						clean = "/_p/" + pluginID + "/admin" + tail
					} else {
						// 尾部尚未带 locale：在 admin 后面插入一次
						// 注意保持单斜杠
						if !strings.HasPrefix(tail, "/") {
							tail = "/" + tail
						}
						clean = "/_p/" + pluginID + "/admin/" + locale + tail
					}
				}
			}
		}
	}

	// (3) 去重复 "/_p/<id>/admin/_p/<id>/admin"
	if pluginID != "" {
		repeated := "/_p/" + pluginID + "/admin/_p/" + pluginID + "/admin"
		for strings.Contains(clean, repeated) {
			clean = strings.ReplaceAll(clean, repeated, "/_p/"+pluginID+"/admin")
		}
		redundant := "/admin/_p/" + pluginID + "/admin"
		for strings.Contains(clean, redundant) {
			clean = strings.ReplaceAll(clean, redundant, "/admin")
		}
	}

	if clean != orig {
		u.Path = clean
		u.RawPath = clean
		resp.Header.Set("Location", u.String())
		resp.Header.Set("X-PX-Location-Rewrite", orig+" -> "+clean) // 诊断头
	}
}

// “像 locale 的段”判断（幂等改写用）
func looksLikeLocaleSegment(seg string) bool {
	if seg == "" {
		return false
	}
	if seg == "__up" {
		return false
	}
	if seg[0] == '_' { // 避免 _nuxt/_ipx 等静态目录
		return false
	}
	if len(seg) > 16 {
		return false
	}
	for _, r := range seg {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func (r *DynamicRouter) buildSignedCtx(c *gin.Context) (ctxB64, sig string, ok bool) {
	if len(r.ctxHMACSecret) == 0 {
		return "", "", false
	}
	claimsAny, exists := c.Get("auth_claims")
	if !exists {
		return "", "", false
	}
	claims, ok := claimsAny.(reqctx.CoreXClaims)
	if !ok {
		return "", "", false
	}
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

func attachTraceHeaders(c *gin.Context, req *http.Request) {
	if c == nil || req == nil {
		return
	}
	_, traceID := getRequestTraceIDs(c)
	if traceID != "" {
		req.Header.Set("X-Trace-Id", traceID)
		req.Header.Set("X-Trace-ID", traceID)
	}
	requestID, _ := getRequestTraceIDs(c)
	if requestID != "" {
		req.Header.Set("X-Request-Id", requestID)
		req.Header.Set("X-Request-ID", requestID)
	}
}

func getRequestTraceIDs(c *gin.Context) (requestID, traceID string) {
	if c == nil {
		return "", ""
	}
	requestID = strings.TrimSpace(c.GetString("request_id"))
	if requestID == "" {
		requestID = strings.TrimSpace(c.GetHeader("X-Request-Id"))
	}
	if requestID == "" {
		requestID = strings.TrimSpace(c.GetHeader("X-Request-ID"))
	}
	traceID = strings.TrimSpace(c.GetString("trace_id"))
	if traceID == "" {
		traceID = strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	}
	if traceID == "" {
		traceID = strings.TrimSpace(c.GetHeader("X-Trace-Id"))
	}
	if traceID == "" {
		traceID = strings.TrimSpace(c.GetHeader("X-Trace-ID"))
	}
	// 兜底：保证网关链路日志不出现空 request_id/trace_id
	if requestID == "" {
		requestID = uuid.NewString()
	}
	if traceID == "" {
		traceID = requestID
	}
	// 回写到上下文与请求头，确保后续日志点保持一致
	c.Set("request_id", requestID)
	c.Set("trace_id", traceID)
	c.Request.Header.Set("X-Request-Id", requestID)
	c.Request.Header.Set("X-Request-ID", requestID)
	c.Request.Header.Set("X-Trace-Id", traceID)
	c.Request.Header.Set("X-Trace-ID", traceID)
	return requestID, traceID
}

func extractUpstreamRequestID(header http.Header, body []byte) string {
	if header != nil {
		v := strings.TrimSpace(header.Get("X-Request-ID"))
		if v != "" {
			return v
		}
		v = strings.TrimSpace(header.Get("X-Request-Id"))
		if v != "" {
			return v
		}
	}
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if v, ok := payload["request_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// 从 "/_p/<id>/api/*" 裁剪为插件侧期望的 client path（保留前导斜杠）
func trimToAPIClientPath(p string) string {
	// 期望形如 "/_p/<id>/api/..."
	if !strings.HasPrefix(p, "/_p/") {
		return p
	}
	rest := strings.TrimPrefix(p, "/_p/")
	i := strings.Index(rest, "/api/")
	if i < 0 {
		return p
	}
	// 仅剥离 "/_p/<id>" 挂载前缀，保留后续完整 client path（含 "/api/v1"）。
	return rest[i:] // 结果以 '/' 开头
}

func isIdentityAuthClientPath(clientPath string) bool {
	path := strings.TrimSpace(clientPath)
	if path == "" {
		return false
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	segs := strings.Split(strings.Trim(path, "/"), "/")
	if len(segs) < 4 {
		return false
	}

	// 允许两类入参：
	// 1) /v1/admin/{identity}/auth/*
	// 2) /api/v1/admin/{identity}/auth/*
	switch {
	case len(segs) >= 5 && segs[0] == "api" && segs[1] == "v1":
		segs = segs[2:]
	case segs[0] == "v1":
		segs = segs[1:]
	}
	if len(segs) < 3 {
		return false
	}
	if segs[0] != "admin" {
		return false
	}
	identity := strings.TrimSpace(segs[1])
	if identity == "" {
		return false
	}
	return segs[2] == "auth"
}

func toHostIdentityAuthPath(clientPath string) string {
	path := strings.TrimSpace(clientPath)
	if path == "" {
		return "/api/v1"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if strings.HasPrefix(path, "/api/v1/") {
		return path
	}
	if strings.HasPrefix(path, "/v1/") {
		return "/api" + path
	}
	if strings.HasPrefix(path, "/admin/") {
		return "/api/v1" + path
	}
	return "/api/v1" + path
}

func normalizeAdminClientPath(pluginID, clientPath string) (string, bool) {
	if clientPath == "" {
		return "/", false
	}
	if pluginID == "" {
		return clientPath, false
	}
	clean := clientPath
	changed := false

	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
		changed = true
	}
	for strings.Contains(clean, "//") {
		clean = strings.ReplaceAll(clean, "//", "/")
		changed = true
	}

	needle := "/_p/" + pluginID + "/admin"
	variants := []string{
		needle,
		"/__up" + needle,
	}

	trimmedOnce := func() bool {
		trimmed := false
		for _, v := range variants {
			if strings.HasPrefix(clean, v) {
				clean = strings.TrimPrefix(clean, v)
				trimmed = true
			}
		}
		if strings.HasPrefix(clean, "/api/") {
			rest := clean[len("/api/"):]
			if idx := strings.Index(rest, needle); idx >= 0 {
				prefix := rest[:idx]
				if !strings.Contains(prefix, "/") { // 只允许一个版本段，例如 v1
					clean = rest[idx+len(needle):]
					trimmed = true
				}
			}
		}
		if trimmed {
			if clean == "" {
				clean = "/"
			}
			if !strings.HasPrefix(clean, "/") {
				clean = "/" + clean
			}
			changed = true
		}
		return trimmed
	}

	for trimmedOnce() {
	}

	if clean == "" {
		clean = "/"
	}
	return clean, changed
}

func (r *DynamicRouter) redirectAdminFromAPI(c *gin.Context, pluginID, clientPath string) bool {
	if pluginID == "" {
		return false
	}
	needle := "/_p/" + pluginID + "/admin"
	idx := strings.Index(clientPath, needle)
	if idx < 0 {
		return false
	}
	tail := clientPath[idx+len(needle):]
	if tail == "" {
		tail = "/"
	}
	if !strings.HasPrefix(tail, "/") {
		tail = "/" + tail
	}
	cleanTail, _ := normalizeAdminClientPath(pluginID, tail)
	dest := joinURLPath(r.basePrefix, pluginID, "admin", cleanTail)
	if q := c.Request.URL.RawQuery; q != "" {
		dest += "?" + q
	}
	logger.InfoF(c.Request.Context(), "[ADMIN-REDIRECT] plugin=%s apiPath=%s => %s", pluginID, clientPath, dest)
	c.Redirect(http.StatusFound, dest)
	c.Abort()
	return true
}

func fixAdminLocationNoLocale(resp *http.Response) {
	if resp.StatusCode < http.StatusMultipleChoices || resp.StatusCode >= http.StatusBadRequest {
		return
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return
	}
	u, err := url.Parse(loc)
	if err != nil || u.Path == "" {
		return
	}

	//clean := strings.ReplaceAll(u.Path, "/__up/_p/", "/_p/")
	clean := u.Path

	// 可选：去掉极端情况下重复段（不涉及 locale）
	// 例如 "/_p/a/admin/_p/a/admin/..." → "/_p/a/admin/..."
	// （如果你不曾遇到，可直接删除下面这段）
	const needle = "/_p/"
	if strings.Count(clean, needle) >= 2 {
		segs := strings.Split(clean, "/")
		// 简单去重：合并相邻的 "/_p/<id>/admin" 重复片段
		out := make([]string, 0, len(segs))
		last := ""
		for _, s := range segs {
			if s == "" {
				continue
			}
			if !(last == "_p" && s == last) {
				out = append(out, s)
			}
			last = s
		}
		clean = "/" + strings.Join(out, "/")
	}

	if clean != u.Path {
		u.Path = clean
		u.RawPath = clean
		resp.Header.Set("Location", u.String())
		resp.Header.Set("X-PX-Location-Rewrite", "strip __up only")
	}
}
