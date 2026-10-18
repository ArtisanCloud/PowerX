package system_recovery

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	manager "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
)

func verifyDatabase(ctx context.Context, dsn string, contract captureContract) error {
	actual, e := inspectDatabase(ctx, dsn)
	if e != nil {
		return e
	}
	if !reflect.DeepEqual(actual.Rows, contract.Rows) || !reflect.DeepEqual(actual.Extensions, contract.Extensions) {
		for table, count := range contract.Rows {
			if actual.Rows[table] != count {
				return fmt.Errorf("数据库表 %s 行数与冻结来源不一致：目标 %d，来源 %d", table, actual.Rows[table], count)
			}
		}
		if !reflect.DeepEqual(actual.Rows, contract.Rows) {
			return errors.New("数据库表集合与冻结来源不一致")
		}
		return errors.New("数据库扩展与冻结来源不一致")
	}
	return nil
}
func (n *Native) Verify(ctx context.Context, d Deployment) (checks []snapshots.RecoveryCheck, result error) {
	checks = []snapshots.RecoveryCheck{}
	file := filepath.Join(d.Root, "launch.yaml")
	verificationServer := false
	if n.Profile.InPlace {
		var spec RunSpec
		if err := snapshots.ReadCoordinatorJSON(filepath.Join(d.Root, "run.json"), &spec); err != nil {
			return nil, err
		}
		if spec.Deployment.Root != d.Root || (spec.Deployment.ConfigFile != d.ConfigFile && spec.Deployment.ConfigFile != file) {
			return nil, errors.New("恢复验证启动配置与固定实例不一致")
		}
		file = spec.Deployment.ConfigFile
		verificationServer = spec.VerificationServer
	}
	if d.SourceManaged {
		file = d.ConfigFile
	}
	raw, e := os.ReadFile(file)
	if e != nil {
		return checks, e
	}
	var cfg config.Config
	if e = yaml.Unmarshal(raw, &cfg); e != nil {
		return checks, e
	}
	socket := ""
	base := fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
	if !d.SourceManaged {
		socket = filepath.Join(d.Root, "core.sock")
		base = "http://recovery.local"
	}
	client := httpClient(socket)
	defer client.CloseIdleConnections()
	get := func(path, bearer string) ([]byte, int, error) {
		request, e := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if e != nil {
			return nil, 0, e
		}
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		response, e := client.Do(request)
		if e != nil {
			return nil, 0, e
		}
		defer response.Body.Close()
		raw, e := io.ReadAll(io.LimitReader(response.Body, 32<<20))
		return raw, response.StatusCode, e
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		if !d.SourceManaged {
			var record processRecord
			if snapshots.ReadCoordinatorJSON(filepath.Join(d.Root, "processes/instance.json"), &record) == nil {
				if _, e := processIdentity(record.PID); e != nil {
					return checks, errors.New(startupFailureReason(d.Root))
				}
			}
		}
		_, code, e := get("/healthz", "")
		if e == nil && code == 200 {
			break
		}
		if time.Now().After(deadline) {
			return checks, errors.New("隔离 Core 未通过健康检查，请查看候选进程日志")
		}
		select {
		case <-ctx.Done():
			return checks, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	redisOptions := &redis.Options{Addr: fmt.Sprintf("%s:%d", cfg.Cache.Host, cfg.Cache.Port), Password: cfg.Cache.Password, DB: cfg.Cache.DB}
	if !d.SourceManaged {
		redisOptions.Network = "unix"
		redisOptions.Addr = filepath.Join(d.Root, "redis.sock")
	}
	cache := redis.NewClient(redisOptions)
	defer cache.Close()
	if e = cache.Ping(ctx).Err(); e != nil {
		return checks, errors.New("候选 Redis 未通过实际连接验证")
	}
	checks = append(checks, snapshots.RecoveryCheck{Name: "redis", Passed: true, Detail: "已加载独立 Redis 持久数据并通过连接验证"})
	checks = append(checks, snapshots.RecoveryCheck{Name: "core_boot", Passed: true, Detail: "匹配版本 Core 实际启动，HTTP 健康检查通过"})
	db, e := pgx.Connect(ctx, cfg.Database.DSN)
	if e != nil {
		return checks, e
	}
	defer db.Close(context.WithoutCancel(ctx))
	// 新签凭据只用于目标校验，不复用生产浏览器 token，也不改变还原的密码。
	var claims reqctx.CoreXClaims
	e = db.QueryRow(ctx, `SELECT u.id,u.uuid::text,m.id,m.uuid::text,m.tenant_uuid::text FROM public.iam_user u JOIN public.iam_member m ON m.user_id=u.id WHERE u.is_root AND u.status=1 AND m.status=1 AND u.deleted_at IS NULL AND m.deleted_at IS NULL ORDER BY m.id LIMIT 1`).Scan(&claims.UserID, &claims.UserUUID, &claims.MemberID, &claims.MemberUUID, &claims.TenantUUID)
	if e != nil {
		return checks, errors.New("没有可用于目标业务验证的有效 Root 成员")
	}
	claims.IsRoot = true
	claims.Platforms = []string{"admin"}
	claims.Env = cfg.Deployment.Env
	if d.SourceManaged {
		// Source instances are read-only inventory: reuse only an already valid
		// session, never create or revoke a source credential.
		e = db.QueryRow(ctx, `SELECT jti,member_uuid FROM public.iam_refresh_token WHERE user_uuid=$1 AND tenant_uuid=$2 AND revoked_at IS NULL AND expires_at>$3 AND deleted_at IS NULL ORDER BY expires_at DESC LIMIT 1`, claims.UserUUID, claims.TenantUUID, time.Now().UnixMilli()).Scan(&claims.ID, &claims.MemberUUID)
		if e != nil {
			return checks, errors.New("来源实例没有可用于业务验证的正常登录会话")
		}
		if claims.MemberUUID == "" {
			claims.MemberID = 0
		}
	} else {
		release, err := prepareVerificationSession(ctx, d, &claims)
		if err != nil {
			return checks, err
		}
		defer func() { result = errors.Join(result, release()) }()
	}
	bearer, e := auth.GenerateAccessJWT(claims, cfg.Auth.Issuer, []string{cfg.Auth.AudienceUser}, 2*time.Minute, []byte(cfg.Auth.JWTSecret))
	if e != nil {
		return checks, e
	}
	prefix := "/" + strings.Trim(cfg.Server.APIPrefix, "/")
	for _, path := range []string{"/admin/user/auth/me/context", "/admin/plugins/menus"} {
		raw, code, e := get(prefix+path, bearer)
		if e != nil || code != 200 {
			return checks, fmt.Errorf("目标身份或菜单业务验证未通过: %s", path)
		}
		var envelope map[string]any
		if json.Unmarshal(raw, &envelope) != nil || envelope["data"] == nil {
			return checks, errors.New("目标业务接口没有返回有效数据")
		}
	}
	checks = append(checks, snapshots.RecoveryCheck{Name: "authenticated_context", Passed: true, Detail: "目标短时 IAM 会话经过实际 Core 身份校验，验证后只撤销本次会话；用户上下文和插件菜单返回有效数据，不替代人工密码登录验收"})
	registry := manager.NewJSONRegistry(cfg.Plugin.RegistryFile)
	if e = registry.Load(ctx); e != nil {
		return checks, e
	}
	for _, plugin := range registry.List(ctx) {
		if plugin.State != plugin_mgr.StateEnabled {
			continue
		}
		probes := 0
		for _, probe := range n.Profile.BusinessProbes {
			if probe.PluginID == plugin.ID {
				probes++
			}
		}
		if probes == 0 {
			return checks, fmt.Errorf("插件 %s 尚未登记业务只读探针，不能仅凭健康检查宣称业务验证通过", plugin.ID)
		}
		var status struct {
			Data struct {
				Runtime struct {
					Healthy bool   `json:"healthy"`
					State   string `json:"state"`
				} `json:"runtime"`
			} `json:"data"`
		}
		for {
			raw, code, e := get(prefix+"/admin/plugins/"+plugin.ID+"/status", bearer)
			if e == nil && code == 200 && json.Unmarshal(raw, &status) == nil && status.Data.Runtime.Healthy {
				break
			}
			if time.Now().After(deadline) {
				return checks, fmt.Errorf("插件 %s 的候选进程未就绪", plugin.ID)
			}
			select {
			case <-ctx.Done():
				return checks, ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
		checks = append(checks, snapshots.RecoveryCheck{Name: "plugin:" + plugin.ID, Passed: true, Detail: "指定版本插件进程已启动且宿主网关运行态健康"})
	}
	for _, probe := range n.Profile.BusinessProbes {
		path := prefix + probe.Path
		if probe.PluginID != "" {
			path = strings.TrimRight(cfg.Plugin.BasePrefix, "/") + "/" + probe.PluginID + probe.Path
		}
		raw, code, e := get(path, bearer)
		if e != nil || code != 200 {
			return checks, fmt.Errorf("业务探针 %s 未通过 (HTTP %d)", probe.Name, code)
		}
		if !validBusinessProbePayload(raw) {
			return checks, fmt.Errorf("业务探针 %s 返回业务错误或空业务数据", probe.Name)
		}

		checks = append(checks, snapshots.RecoveryCheck{Name: "business:" + probe.Name, Passed: true, Detail: "通过实际受保护的业务读取接口验证"})
	}
	// 所有媒体已在物化阶段逐一校验；再通过真实资源 API 验证本租户可见文件。
	var table string
	e = db.QueryRow(ctx, `SELECT quote_ident(schemaname)||'.'||quote_ident(tablename) FROM pg_tables WHERE tablename='media_assets' AND schemaname NOT IN ('pg_catalog','information_schema') LIMIT 1`).Scan(&table)
	mediaCount := 0
	if e == nil {
		rows, e := db.Query(ctx, "SELECT uuid::text,expected_checksum FROM "+table+" WHERE tenant_uuid=$1 AND upload_state='ready' AND deleted_at IS NULL ORDER BY id LIMIT 3", claims.TenantUUID)
		if e != nil {
			return checks, e
		}
		type reference struct{ id, sha string }
		refs := []reference{}
		for rows.Next() {
			var r reference
			if e = rows.Scan(&r.id, &r.sha); e != nil {
				rows.Close()
				return checks, e
			}
			refs = append(refs, r)
		}
		rows.Close()
		for _, r := range refs {
			raw, code, e := get(prefix+"/admin/media/assets/"+r.id+"/resource", bearer)
			sum := sha256.Sum256(raw)
			if e != nil || code != 200 || hex.EncodeToString(sum[:]) != strings.ToLower(r.sha) {
				return checks, errors.New("目标媒体资源 API 的内容与数据库登记摘要不一致")
			}
			mediaCount++
		}
	}
	checks = append(checks, snapshots.RecoveryCheck{Name: "media_api", Passed: true, Detail: fmt.Sprintf("全包文件摘要已校验，实际媒体资源 API 抽验 %d 个当前租户文件", mediaCount)})
	webSocket := ""
	webURL := fmt.Sprintf("http://127.0.0.1:%d", cfg.WebAdminPort)
	if !d.SourceManaged && (cfg.Recovery.ReadOnly() || verificationServer) {
		webSocket = filepath.Join(d.Root, "web.sock")
		webURL = "http://recovery.local"
	}
	webClient := httpClient(webSocket)
	defer webClient.CloseIdleConnections()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, webURL+"/", nil)
	response, e := webClient.Do(request)
	if e != nil {
		return checks, errors.New("Web Admin 未在候选环境启动")
	}
	_ = response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 400 {
		return checks, errors.New("Web Admin 页面未通过检查")
	}
	checks = append(checks, snapshots.RecoveryCheck{Name: "web_admin", Passed: true, Detail: "恢复包内 Web Admin 产物实际运行并返回页面"})
	return checks, nil
}

func startupFailureReason(root string) string {
	file, e := os.Open(filepath.Join(root, "processes/instance.json.log"))
	if e != nil {
		return "候选启动进程已退出，请查看候选进程日志"
	}
	defer file.Close()
	// Only a curated validator marker is safe to expose; never return raw logs.
	scanner := bufio.NewScanner(io.LimitReader(file, 8<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	reason := "候选启动进程已退出，请查看候选进程日志"
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "RECOVERY_STARTUP_BLOCKED: RECOVERY_") && len(line) < 700 {
			reason = "恢复配置校验未通过：" + strings.TrimPrefix(line, "RECOVERY_STARTUP_BLOCKED: ")
		}
	}
	return reason
}

func validBusinessProbePayload(raw []byte) bool {
	var payload any
	if json.Unmarshal(raw, &payload) != nil || payload == nil {
		return false
	}
	if envelope, ok := payload.(map[string]any); ok {
		if success, present := envelope["success"]; present && success != true {
			return false
		}
		if failure, present := envelope["error"]; present && failure != nil && failure != "" {
			return false
		}
		if code, ok := envelope["code"].(float64); ok && code != 0 && code != 200 {
			return false
		}
		if data, present := envelope["data"]; present && data == nil {
			return false
		}
	}
	return true
}
