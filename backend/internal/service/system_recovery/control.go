package system_recovery

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

// The stable gateway retains the current recovery control protocol even when an
// older Core/UI is restored. Every control request requires a live Root USER
// session checked by that active Core. Progress tickets never authorize control.
func (c *Coordinator) control(w http.ResponseWriter, r *http.Request, a Active) bool {
	file := filepath.Join(a.Deployment.Root, "launch.yaml")
	if a.Deployment.SourceManaged {
		file = a.Deployment.ConfigFile
	}
	// Fixed installations always launch their authoritative config. launch.yaml
	// is only a temporary read-only verification config in the new workflow.
	if native, ok := c.Driver.(*Native); ok && native.Profile.InPlace {
		file = a.Deployment.ConfigFile
	}
	raw, e := os.ReadFile(file)
	var cfg config.Config
	if e != nil || yaml.Unmarshal(raw, &cfg) != nil {
		return false
	}
	prefix := "/" + strings.Trim(cfg.Server.APIPrefix, "/")
	base := prefix + "/admin/ops/backup/system-snapshots"
	path := strings.TrimPrefix(r.URL.Path, base)
	if path == r.URL.Path {
		return false
	}
	segments := strings.Split(strings.Trim(path, "/"), "/")
	overview := r.Method == "GET" && path == "/recovery"
	prepare := r.Method == "POST" && len(segments) == 2 && segments[1] == "recovery"
	action := r.Method == "POST" && len(segments) == 3 && segments[0] == "recovery" && (segments[2] == "cutover" || segments[2] == "rollback" || segments[2] == "cleanup")
	if !overview && !prepare && !action {
		return false
	}
	if action {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 410, "message": "环境切换接口已停用；请选择快照恢复当前实例，需要回退时恢复保护快照"})
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, message string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": status, "message": message})
	}
	bearer := r.Header.Get("Authorization")
	if !strings.HasPrefix(bearer, "Bearer ") {
		fail(401, "请使用当前站点 Root 用户登录")
		return true
	}
	socket := ""
	host := "http://127.0.0.1:" + strconv.Itoa(cfg.Server.Port)
	if !a.Deployment.SourceManaged {
		socket = filepath.Join(a.Deployment.Root, "core.sock")
		host = "http://recovery.local"
	}
	client := httpClient(socket)
	defer client.CloseIdleConnections()
	request, _ := http.NewRequestWithContext(r.Context(), "GET", host+prefix+"/admin/user/auth/me/context", nil)
	request.Header.Set("Authorization", bearer)
	response, e := client.Do(request)
	if e != nil {
		fail(503, "当前身份服务尚未就绪；独立进度仍可查看")
		return true
	}
	defer response.Body.Close()
	var identity struct {
		Data struct {
			IsRoot bool `json:"is_root"`
			User   struct {
				ID     uint64 `json:"id"`
				IsRoot bool   `json:"is_root"`
			} `json:"user"`
		} `json:"data"`
	}
	if response.StatusCode != 200 {
		status := response.StatusCode
		if status != 401 && status != 403 {
			status = 503
		}
		fail(status, "请在恢复后的当前站点重新验证 Root 登录")
		return true
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&identity) != nil || !identity.Data.IsRoot || !identity.Data.User.IsRoot || identity.Data.User.ID == 0 {
		fail(403, "仅 Root 用户可操作整站恢复")
		return true
	}
	claims := &reqctx.CoreXClaims{IsRoot: true, UserID: identity.Data.User.ID}
	claims.Subject = fmt.Sprintf("user:%d", identity.Data.User.ID)
	ctx := reqctx.WithClaims(reqctx.WithIsRoot(r.Context(), true), claims)
	if _, e = c.issueRecoverySession(w, r, claims.Subject, "root"); e != nil {
		fail(503, "无法准备独立恢复会话")
		return true
	}
	if overview {
		state, e := c.Store.EnvironmentState()
		if e != nil {
			fail(500, "无法读取环境状态")
			return true
		}
		jobs, e := c.Store.List()
		if e != nil {
			fail(500, "无法读取恢复任务")
			return true
		}
		tokens := map[string]string{}
		for _, job := range jobs {
			token, e := c.Store.ProgressToken(job.ID)
			if e != nil {
				fail(500, "进度票据不可用")
				return true
			}
			tokens[job.ID] = token
		}
		state["configured"] = true
		state["online"] = c.Store.Online()
		state["targets"] = c.Store.Targets
		state["recovery_entry"] = recoveryEntry
		state["jobs"] = jobs
		state["progress_tokens"] = tokens
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": state})
		return true
	}
	var input struct {
		Confirmation string `json:"confirmation"`
		TargetID     string `json:"target_id"`
		Mode         string `json:"restore_mode"`
		Generation   uint64 `json:"generation"`
	}
	decode := json.NewDecoder(io.LimitReader(r.Body, 4096))
	decode.DisallowUnknownFields()
	if e = decode.Decode(&input); e != nil {
		fail(400, "恢复请求无效")
		return true
	}
	var extra any
	if decode.Decode(&extra) != io.EOF {
		fail(400, "只接受单个恢复请求")
		return true
	}
	id := segments[0]
	if action {
		id = segments[1]
	}
	if input.Confirmation != id {
		fail(400, "请确认具体快照或环境")
		return true
	}
	if !c.Store.Online() {
		fail(503, "独立协调器未在线")
		return true
	}
	var job snapshots.ManagedJob
	if prepare {
		if service := c.portalSnapshots(); service != nil {
			snapshot, err := service.Get(ctx, id)
			if err != nil || !c.portalPackageAllowed(snapshot) {
				fail(409, "此快照不可用或不符合当前实例恢复合同")
				return true
			}
		}
		if input.TargetID != "" || input.Mode != "" {
			fail(400, "恢复仅替换当前实例，不再接受目标环境或切换模式")
			return true
		}
		job, e = c.Store.EnqueueInstanceRestore(ctx, id, r.Header.Get("Idempotency-Key"), input.Generation)
	} else {
		switch segments[2] {
		case "cutover":
			job, e = c.Store.EnqueueCutover(ctx, id, r.Header.Get("Idempotency-Key"), input.Generation)
		case "rollback":
			job, e = c.Store.EnqueueRollback(ctx, id, r.Header.Get("Idempotency-Key"), input.Generation)
		case "cleanup":
			job, e = c.Store.EnqueueCleanup(ctx, id, r.Header.Get("Idempotency-Key"), input.Generation)
		}
	}
	if e != nil {
		status := 500
		message := "恢复请求失败"
		var app *dto.AppError
		if errors.As(e, &app) {
			status = app.HTTPCode
			message = app.Message
		}
		fail(status, message)
		return true
	}
	token, e := c.Store.ProgressToken(job.ID)
	if e != nil {
		fail(500, "无法创建进度票据")
		return true
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 202, "data": map[string]any{"recovery": job, "progress_token": token}})
	return true
}
