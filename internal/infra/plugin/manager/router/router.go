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
	target   *url.URL // e.g. http://127.0.0.1:31001
	basePath string   // e.g. "/v1" (插件 manifest.endpoints.http_base_path)
}

type DynamicRouter struct {
	basePrefix string
	engine     *gin.Engine

	mu        sync.RWMutex
	adminDirs map[string]string      // pluginID -> abs dir
	apis      map[string]apiUpstream // pluginID -> upstream

	gate *authzGate
}

func NewDynamicRouter(basePrefix string, engine *gin.Engine) *DynamicRouter {
	dr := &DynamicRouter{
		basePrefix: basePrefix,
		engine:     engine,
		adminDirs:  make(map[string]string),
		apis:       make(map[string]apiUpstream),
	}
	grp := engine.Group(basePrefix)
	// Admin 静态
	grp.GET("/:id/admin/*filepath", dr.serveAdminStatic)
	grp.HEAD("/:id/admin/*filepath", dr.serveAdminStatic)
	// API 反代
	grp.Any("/:id/api/*filepath", dr.serveAPIProxy)
	return dr
}

func (r *DynamicRouter) MountAdminStatic(id, absDir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adminDirs[id] = absDir
}

func (r *DynamicRouter) MountAPIProxy(id string, upstream *url.URL, basePath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// 规范化 basePath
	if basePath == "" {
		basePath = "/"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	r.apis[id] = apiUpstream{target: upstream, basePath: basePath}
}

func (r *DynamicRouter) Unmount(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.adminDirs, id)
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

	// 从认证中间件取 claims（或自行解析入站JWT）
	var claims reqctx.CoreXClaims
	if v, ok := c.Get("auth_claims"); ok {
		if cc, ok := v.(reqctx.CoreXClaims); ok {
			claims = cc
		}
	}

	// 预检 + 换签短期 Token
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
		// 交给默认 director 处理 scheme/host，再改 path
		if origDirector != nil {
			origDirector(req)
		} else {
			req.URL.Scheme = up.target.Scheme
			req.URL.Host = up.target.Host
		}

		// 拼接下游路径： upstreamBase + manifestBasePath + clientPath
		reqPath := joinURLPath(up.target.Path, up.basePath, clientPath)
		req.URL.Path = reqPath
		req.URL.RawPath = reqPath

		// 覆盖授权头为“插件内部短期 Token”
		req.Header.Del("Authorization")
		if pluginToken != "" {
			req.Header.Set("Authorization", "Bearer "+pluginToken)
		}

		// 透传签名上下文（供插件兜底/审计）
		if ctxB64, sig, ok := buildSignedCtx(c); ok {
			req.Header.Set("X-PowerX-CTX", ctxB64)
			req.Header.Set("X-PowerX-CTX-SIG", sig)
		}

		// Host 头设置为下游
		req.Host = up.target.Host
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func buildSignedCtx(c *gin.Context) (ctxB64, sig string, ok bool) {
	claimsAny, exists := c.Get("auth_claims")
	if !exists {
		return "", "", false
	}
	claims := claimsAny.(reqctx.CoreXClaims)

	raw, _ := json.Marshal(claims)
	ctxB64 = base64.StdEncoding.EncodeToString(raw)

	// HMAC 秘钥（示例）：建议换成配置读取
	const pluginCtxHMACSecret = "pluginCtxHMACSecret"
	mac := hmac.New(sha256.New, []byte(pluginCtxHMACSecret))
	mac.Write([]byte(ctxB64))
	sig = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return ctxB64, sig, true
}

// joinURLPath 安全拼接 URL 路径片段（保留单个斜杠分隔）
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
