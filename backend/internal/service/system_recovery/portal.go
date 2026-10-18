package system_recovery

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/eventstream"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// 维护页面嵌入恢复执行程序；普通 Core exec 为维护模式时仍从原端口提供。
//
//go:embed portal/index.html portal/app.js portal/style.css
var recoveryAssets embed.FS

const recoveryEntry = "/__powerx_recovery/"

func portalJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func portalError(w http.ResponseWriter, status int, message string) {
	portalJSON(w, status, map[string]string{"message": message})
}

func (c *Coordinator) serveRecoveryPage(w http.ResponseWriter, r *http.Request) {
	asset := "index.html"
	if r.URL.Path == recoveryEntry+"app.js" {
		asset = "app.js"
	}
	if r.URL.Path == recoveryEntry+"style.css" {
		asset = "style.css"
	}
	raw, err := recoveryAssets.ReadFile("portal/" + asset)
	if err != nil {
		http.Error(w, "恢复页面不可用", 500)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(asset)))
	if r.Method != "HEAD" {
		_, _ = w.Write(raw)
	}
}
func (c *Coordinator) portalSnapshots() *snapshots.Service {
	if c.SnapshotStore != nil {
		return c.SnapshotStore
	}
	if native, ok := c.Driver.(*Native); ok {
		return native.SnapshotStore
	}
	return nil
}
func recoveryContext(ctx context.Context, operator string) context.Context {
	claims := &reqctx.CoreXClaims{IsRoot: true}
	claims.Subject = operator
	return reqctx.WithIsRoot(reqctx.WithClaims(ctx, claims), true)
}
func decodePortalJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		portalError(w, 415, "请使用 JSON 请求")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		portalError(w, 400, "恢复请求格式无效")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		portalError(w, 400, "恢复请求只能包含一个对象")
		return false
	}
	return true
}

// portal is authenticated without Core. Task tickets remain read-only and are
// obtained after authentication; neither ticket nor administrator key is a URL.
func (c *Coordinator) portal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.RawQuery != "" {
		portalError(w, 400, "恢复入口不接受 URL 参数或凭据")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, recoveryEntry)
	if (path == "" || path == "app.js" || path == "style.css") && (r.Method == "GET" || r.Method == "HEAD") {
		c.serveRecoveryPage(w, r)
		return
	}
	if path == "health" && r.Method == "GET" {
		portalJSON(w, 200, map[string]any{"available": true, "protocol": 1})
		return
	}
	if path == "session" && r.Method == "POST" {
		if !recoverySameOrigin(r) {
			portalError(w, 403, "恢复认证必须来自当前站点")
			return
		}
		if !c.allowPortalLogin() {
			w.Header().Set("Retry-After", "60")
			portalError(w, 429, "认证过于频繁，请稍后重试")
			return
		}
		if _, err := c.EnsureRecoveryAccess(); err != nil {
			portalError(w, 503, "恢复认证尚未就绪")
			return
		}
		var input struct {
			Key string `json:"key"`
		}
		if !decodePortalJSON(w, r, &input) {
			return
		}
		operator, method := "recovery-administrator", "recovery-key"
		if input.Key == "" {
			active, err := c.Active()
			if err != nil {
				portalError(w, 503, "当前实例记录不可读")
				return
			}
			claims, status, err := c.liveRecoveryRoot(r, active)
			if err != nil {
				portalError(w, status, err.Error())
				return
			}
			operator, method = claims.Subject, "root"
		} else if !c.verifyRecoveryKey(strings.TrimSpace(input.Key)) {
			c.portalAudit("session.denied", "anonymous", "invalid recovery key")
			portalError(w, 401, "恢复密钥无效")
			return
		}
		session, err := c.issueRecoverySession(w, r, operator, method)
		if err != nil {
			portalError(w, 503, "无法创建恢复管理员会话")
			return
		}
		portalJSON(w, 200, map[string]any{"authenticated": true, "csrf": session.CSRF, "operator": session.Operator, "expires_at": session.ExpiresAt})
		return
	}
	session, err := c.readRecoverySession(r)
	if err != nil {
		portalError(w, 401, "请验证恢复管理员身份；Core 停机时可使用恢复密钥")
		return
	}
	if path == "session" && r.Method == "GET" {
		portalJSON(w, 200, map[string]any{"authenticated": true, "csrf": session.CSRF, "operator": session.Operator, "expires_at": session.ExpiresAt})
		return
	}
	if r.Method != "GET" && !recoveryCSRF(r, session) {
		portalError(w, 403, "恢复确认已失效，请刷新后重新确认")
		return
	}
	if path == "session" && r.Method == "DELETE" {
		if err = os.Remove(c.sessionFile(session.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			portalError(w, 500, "无法退出恢复会话")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: recoveryCookie, Value: "", Path: "/", HttpOnly: true, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteStrictMode, MaxAge: -1})
		c.portalAudit("session.deleted", session.Operator, session.Method)
		portalJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if path == "recovery-key" && r.Method == "POST" {
		key, err := c.readRecoveryKey()
		if err != nil {
			portalError(w, 503, "恢复密钥不可用")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="powerx-recovery-administrator.key"`)
		c.portalAudit("key.downloaded", session.Operator, session.Method)
		_, _ = io.WriteString(w, key+"\n")
		return
	}
	if path == "catalog" && r.Method == "GET" {
		c.portalCatalog(w, r, session)
		return
	}
	if path == "events" && r.Method == "GET" {
		service := c.portalSnapshots()
		if service == nil {
			portalError(w, 503, "快照目录尚未登记")
			return
		}
		view, err := service.Overview(recoveryContext(r.Context(), session.Operator))
		if err != nil {
			portalError(w, 503, "无法读取快照目录")
			return
		}
		sub, err := opswatch.Subscribe(r.Context(), map[string][]string{"restore": {filepath.Join(c.Store.Directory, "jobs")}, "snapshots": {filepath.Join(view.Directory, "snapshots")}}, nil)
		if err != nil {
			portalError(w, 503, "恢复状态通知不可用")
			return
		}
		defer sub.Close()
		_ = eventstream.Serve(r.Context(), w, sub, func([]string) (eventstream.Event, error) {
			if _, err := c.readRecoverySession(r); err != nil {
				return eventstream.Event{}, err
			}
			return eventstream.Event{Name: "invalidate", Data: map[string]bool{"changed": true}}, nil
		})
		return
	}
	if (path == "restore" || path == "resume") && r.Method == "POST" {
		c.portalRestore(w, r, session, path)
		return
	}
	portalError(w, 404, "恢复操作不存在")
}

func (c *Coordinator) portalCatalog(w http.ResponseWriter, r *http.Request, session recoverySession) {
	active, err := c.Active()
	if err != nil {
		portalError(w, 503, "当前实例记录不可读")
		return
	}
	service := c.portalSnapshots()
	if service == nil {
		portalError(w, 503, "快照目录尚未登记")
		return
	}
	view, err := service.Overview(recoveryContext(r.Context(), session.Operator))
	if err != nil {
		portalError(w, 503, "无法读取快照目录")
		return
	}
	jobs, err := c.Store.List()
	if err != nil {
		portalError(w, 503, "无法读取独立恢复记录")
		return
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAt.After(jobs[j].CreatedAt) })
	if len(jobs) > 100 {
		jobs = jobs[:100]
	}
	tokens := map[string]string{}
	for i := range jobs {
		jobs[i].RequestHash = ""
		if ticket, e := c.Store.ProgressToken(jobs[i].ID); e == nil {
			tokens[jobs[i].ID] = ticket
		}
	}
	packages := make([]map[string]any, 0, len(view.Snapshots))
	for _, snapshot := range view.Snapshots {
		var size int64
		for _, component := range snapshot.Components {
			size += component.Size
		}
		packages = append(packages, map[string]any{"id": snapshot.ID, "created_at": snapshot.CreatedAt, "captured_at": snapshot.CapturedAt, "version": snapshot.Version, "status": snapshot.Status, "protected": snapshot.Protected, "size_bytes": size, "allowed": c.portalPackageAllowed(snapshot), "error": snapshot.Error})
	}
	var startup struct {
		Error string `json:"error"`
	}
	_ = snapshots.ReadCoordinatorJSON(filepath.Join(c.Store.Directory, "startup-error.json"), &startup)
	executorError := c.executorProblem()
	portalJSON(w, 200, map[string]any{"generation": active.Generation, "maintenance": active.Maintenance, "job_id": active.JobID, "snapshots": packages, "jobs": jobs, "progress_tokens": tokens, "startup_error": startup.Error, "business_online": c.businessOnline(r.Context(), active.Deployment), "executor_online": c.Store.Online() && executorError == "", "executor_error": executorError, "restore_problem": c.Store.RestoreProblem})
}
func (c *Coordinator) portalPackageAllowed(snapshot snapshots.Snapshot) bool {
	if snapshot.Status != "ready" {
		return false
	}
	for _, target := range c.Store.Targets {
		if target.AllowOverwrite && (snapshot.RecoverySupported || (target.Environment == "dev" && target.AllowOnlineData)) {
			return true
		}
	}
	return false
}
func (c *Coordinator) portalRestore(w http.ResponseWriter, r *http.Request, session recoverySession, action string) {
	if !c.Store.Online() {
		portalError(w, 503, "恢复执行服务尚未就绪，请刷新状态后重试")
		return
	}
	if c.executorProblem() != "" {
		portalError(w, 503, "恢复执行器出现错误，请运维核对后重启恢复服务")
		return
	}
	var input struct {
		SnapshotID   string `json:"snapshot_id"`
		JobID        string `json:"job_id"`
		Confirmation string `json:"confirmation"`
		Generation   uint64 `json:"generation"`
	}
	if !decodePortalJSON(w, r, &input) {
		return
	}
	ctx := recoveryContext(r.Context(), session.Operator)
	var job snapshots.ManagedJob
	var err error
	if action == "restore" {
		if uuid.Validate(input.SnapshotID) != nil || input.Confirmation != input.SnapshotID || input.JobID != "" {
			portalError(w, 400, "请确认所选快照")
			return
		}
		service := c.portalSnapshots()
		if service == nil {
			portalError(w, 503, "快照目录尚未登记")
			return
		}
		snapshot, e := service.Get(ctx, input.SnapshotID)
		if e != nil || !c.portalPackageAllowed(snapshot) {
			portalError(w, 409, "所选快照不可用或不符合当前实例恢复合同")
			return
		}
		job, err = c.Store.EnqueueInstanceRestore(ctx, input.SnapshotID, r.Header.Get("Idempotency-Key"), input.Generation)
	} else {
		if uuid.Validate(input.JobID) != nil || input.Confirmation != input.JobID || input.SnapshotID != "" {
			portalError(w, 400, "请确认需要继续的当前实例任务")
			return
		}
		job, err = c.Store.EnqueueResume(ctx, input.JobID, r.Header.Get("Idempotency-Key"), input.Generation)
	}
	if err != nil {
		portalError(w, 409, err.Error())
		return
	}
	ticket, err := c.Store.ProgressToken(job.ID)
	if err != nil {
		portalError(w, 503, "无法签发独立进度票据")
		return
	}
	c.portalAudit("instance."+action, session.Operator, job.ID)
	portalJSON(w, 202, map[string]any{"recovery": job, "progress_token": ticket})
}

func (c *Coordinator) executorProblem() string {
	if value := c.executorFailure.Load(); value != nil && value.(string) != "" {
		return value.(string)
	}
	var failure struct {
		Error string `json:"error"`
	}
	_ = snapshots.ReadCoordinatorJSON(filepath.Join(c.Store.Directory, "executor-error.json"), &failure)
	return failure.Error
}

func deploymentRecoveryConfig(d Deployment) (*config.Config, error) {
	file := filepath.Join(d.Root, "launch.yaml")
	if d.SourceManaged || d.ConfigFile == filepath.Join(d.Root, "etc/config.yaml") {
		file = d.ConfigFile
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var cfg config.Config
	err = yaml.Unmarshal(raw, &cfg)
	return &cfg, err
}
func (c *Coordinator) liveRecoveryRoot(r *http.Request, active Active) (*reqctx.CoreXClaims, int, error) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return nil, 401, errors.New("请从正常站点 Root 会话进入，或使用恢复密钥")
	}
	cfg, err := deploymentRecoveryConfig(active.Deployment)
	if err != nil {
		return nil, 503, errors.New("身份服务暂不可用，请使用恢复密钥")
	}
	socket := ""
	host := "http://127.0.0.1:" + strconv.Itoa(cfg.Server.Port)
	if !active.Deployment.SourceManaged {
		socket = filepath.Join(active.Deployment.Root, "core.sock")
		host = "http://recovery.local"
	}
	client := httpClient(socket)
	defer client.CloseIdleConnections()
	client.Timeout = 5 * time.Second
	req, _ := http.NewRequestWithContext(r.Context(), "GET", host+"/"+strings.Trim(cfg.Server.APIPrefix, "/")+"/admin/user/auth/me/context", nil)
	req.Header.Set("Authorization", r.Header.Get("Authorization"))
	response, err := client.Do(req)
	if err != nil {
		return nil, 503, errors.New("Core 已停止或身份服务不可用，请使用恢复密钥")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, 403, errors.New("仅当前站点 Root 管理员可进入恢复控制台")
	}
	var identity struct {
		Data struct {
			IsRoot bool `json:"is_root"`
			User   struct {
				ID     uint64 `json:"id"`
				IsRoot bool   `json:"is_root"`
			} `json:"user"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&identity) != nil || !identity.Data.IsRoot || !identity.Data.User.IsRoot || identity.Data.User.ID == 0 {
		return nil, 403, errors.New("仅当前站点 Root 管理员可进入恢复控制台")
	}
	claims := &reqctx.CoreXClaims{IsRoot: true, UserID: identity.Data.User.ID}
	claims.Subject = fmt.Sprintf("user:%d", identity.Data.User.ID)
	return claims, 200, nil
}
func (c *Coordinator) businessOnline(ctx context.Context, d Deployment) bool {
	cfg, err := deploymentRecoveryConfig(d)
	if err != nil {
		return false
	}
	socket := ""
	host := "http://127.0.0.1:" + strconv.Itoa(cfg.Server.Port)
	if !d.SourceManaged {
		socket = filepath.Join(d.Root, "core.sock")
		host = "http://recovery.local"
	}
	client := httpClient(socket)
	defer client.CloseIdleConnections()
	client.Timeout = 350 * time.Millisecond
	request, _ := http.NewRequestWithContext(ctx, "GET", host+"/healthz", nil)
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == 200
}
