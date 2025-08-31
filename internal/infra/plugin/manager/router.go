package manager

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

type apiUpstream struct {
	target   *url.URL // e.g. http://127.0.0.1:31001
	basePath string   // e.g. "/v1" (来自 manifest.endpoints.http_base_path)
}

type DynamicRouter struct {
	basePrefix string
	engine     *gin.Engine

	mu        sync.RWMutex
	adminDirs map[string]string      // pluginID -> abs dir
	apis      map[string]apiUpstream // pluginID -> upstream
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
	r.apis[id] = apiUpstream{target: upstream, basePath: basePath}
}
func (r *DynamicRouter) Unmount(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.adminDirs, id)
	delete(r.apis, id)
}

func (r *DynamicRouter) serveAdminStatic(c *gin.Context) {
	id := c.Param("id")
	rel := c.Param("filepath")
	if rel == "" || rel == "/" {
		rel = "/index.html"
	}
	r.mu.RLock()
	base, ok := r.adminDirs[id]
	r.mu.RUnlock()
	if !ok || base == "" {
		c.Status(http.StatusNotFound)
		return
	}
	clean := filepath.Clean(strings.TrimPrefix(rel, "/"))
	target := filepath.Join(base, clean)
	if !isSubPath(base, target) {
		c.Status(http.StatusForbidden)
		return
	}
	if _, err := os.Stat(target); os.IsNotExist(err) {
		target = filepath.Join(base, "index.html") // SPA 回退
	}
	http.ServeFile(c.Writer, c.Request, target)
}

func buildSignedCtx(c *gin.Context) (ctxB64, sig string, ok bool) {
	claimsAny, exists := c.Get("auth_claims") // 你的鉴权中间件设进去的
	if !exists {
		return "", "", false
	}
	claims := claimsAny.(auth.CoreXClaims)

	raw, _ := json.Marshal(claims)
	ctxB64 = base64.StdEncoding.EncodeToString(raw)

	mac := hmac.New(sha256.New, []byte("pluginCtxHMACSecret"))
	mac.Write([]byte(ctxB64))
	sig = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return ctxB64, sig, true
}

func (r *DynamicRouter) serveAPIProxy(c *gin.Context) {
	id := c.Param("id")

	r.mu.RLock()
	up, ok := r.apis[id]
	r.mu.RUnlock()
	if !ok || up.target == nil {
		// 没有为该插件挂 API 上游，直接 404（你日志里看到的就是这行）
		c.Status(http.StatusNotFound)
		return
	}

	// 反代到插件：只使用客户端传来的 *filepath（形如 "/v1/ping"）
	// 不再额外拼接 basePath，避免出现 /v1/v1/ping
	clientPath := c.Param("filepath") // 保留前导 "/"
	if clientPath == "" {
		clientPath = "/"
	}

	proxy := httputil.NewSingleHostReverseProxy(up.target)
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = up.target.Scheme
		req.URL.Host = up.target.Host
		req.Host = up.target.Host
		// 安全拼接 upstream.Path 与客户端路径
		req.URL.Path = joinURLPath(up.target.Path, clientPath)
		// 可选：转发原查询串
		req.URL.RawQuery = c.Request.URL.RawQuery
		// 1) 清洗潜在伪造
		req.Header.Del("X-PowerX-CTX")
		req.Header.Del("X-PowerX-CTX-SIG")

		// 2) 注入签名上下文
		if ctxB64, sig, ok := buildSignedCtx(c); ok {
			req.Header.Set("X-PowerX-CTX", ctxB64)
			req.Header.Set("X-PowerX-CTX-SIG", sig)
		}

		// 3) 可选：透传追踪信息
		if tid := c.GetString("trace_id"); tid != "" {
			req.Header.Set("X-Request-ID", tid)
		}
	}
	proxy.ServeHTTP(c.Writer, c.Request)
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
