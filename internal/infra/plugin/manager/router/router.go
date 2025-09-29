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
	"unicode"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/security"
	"github.com/gin-gonic/gin"
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

func NewDynamicRouter(basePrefix string, engine *gin.Engine) *DynamicRouter {
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

		// API 建议用 Any 覆盖所有方法
		grp.Any("/:id/api/*filepath", dr.serveAPIProxy)
	}

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
	if len(r.ctxHMACSecret) == 0 {
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
			// —— 只做传球：/_p/<id>/admin/*filepath（不做任何 locale 注入/剥离）
			req.URL.Path = joinURLPath(r.basePrefix, pluginID, "admin", clientPath)
			req.URL.RawPath = req.URL.Path

			// 仅传递宿主挂载点（不含 locale）
			req.Host = up.target.Host
			req.Header.Set("X-Forwarded-Host", c.Request.Host)
			req.Header.Set("X-Forwarded-Proto", "http")
			req.Header.Set("X-Forwarded-Prefix", joinURLPath(r.basePrefix, pluginID, "admin")) // "/_p/<id>/admin"

			// 诊断
			req.Header.Set("X-PX-Upstream", up.target.String())
			req.Header.Set("X-PX-Client-Path", clientPath)
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

	applyAdminFrameHeaders(c.Writer.Header())
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

	// 认证上下文（可选）
	var claims reqctx.CoreXClaims
	if v, ok := c.Get("auth_claims"); ok {
		if cc, ok := v.(reqctx.CoreXClaims); ok {
			claims = cc
		}
	}

	// 预检 + 下发短期 Token（可选）
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

		// 只做传球：把 "/_p/<id>/api/*" 裁剪为插件真实 basePath
		trimmed := trimToAPIClientPath(joinURLPath(r.basePrefix, pluginID, "api", clientPath))
		var reqPath string
		if up.basePath != "" && up.basePath != "/" && strings.HasPrefix(trimmed, up.basePath) {
			reqPath = joinURLPath(up.target.Path, trimmed)
		} else {
			reqPath = joinURLPath(up.target.Path, up.basePath, trimmed)
		}
		req.URL.Path = reqPath
		req.URL.RawPath = reqPath

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
	return rest[i+len("/api/")-1:] // 保留前导 '/'
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
