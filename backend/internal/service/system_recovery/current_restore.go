package system_recovery

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
)

type currentJournal struct {
	JobID           string        `json:"job_id"`
	DatabaseTouched bool          `json:"database_touched"`
	RedisTouched    bool          `json:"redis_touched"`
	Files           []currentFile `json:"files"`
}
type currentFile struct {
	Target   string `json:"target"`
	Prepared string `json:"prepared"`
	Backup   string `json:"backup"`
	Started  bool   `json:"started"`
	Existed  bool   `json:"existed"`
}

func (r *CurrentRuntime) work(id string) string { return filepath.Join(r.spec.Directory, "work", id) }
func (r *CurrentRuntime) journalFile(id string) string {
	return filepath.Join(r.work(id), "journal.json")
}
func (r *CurrentRuntime) phase(j *snapshots.ManagedJob, phase string) error {
	j.Phase = phase
	return r.Coordinator.Store.Save(*j)
}

func (r *CurrentRuntime) executeCurrentPending(ctx context.Context) (bool, error) {
	jobs, err := r.Coordinator.Store.List()
	if err != nil {
		return false, err
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAt.Before(jobs[j].CreatedAt) })
	for _, j := range jobs {
		if j.Status != "queued" && j.Status != "running" {
			continue
		}
		if j.Kind != "restore" && j.Kind != "resume" {
			continue
		}
		if j.Kind == "resume" {
			j.Status, j.Phase = "running", "verifying_startup"
			j.WritesMayHaveResumed = true
			if err = r.Coordinator.Store.Save(j); err != nil {
				return false, err
			}
			return true, nil
		}
		if j.Phase == "verifying_startup" {
			j.Status = "attention_required"
			j.Error = "正常启动未完成，已保留维护现场和保护快照；请查看报告后继续当前实例"
			_ = r.Coordinator.Store.Save(j)
			return false, nil
		}
		active, err := r.Coordinator.Active()
		if err != nil {
			return false, err
		}
		if j.Status == "running" {
			// 断电后不在半恢复状态启动业务；先用持久日志与保护快照补偿。
			err = r.compensateCurrent(ctx, &j)
			if err != nil {
				j.Status = "attention_required"
				j.Error = "恢复中断，自动补偿未完成；请检查维护报告"
				_ = r.Coordinator.Store.Save(j)
				return false, err
			}
			j.Status, j.Phase, j.Error = "rolled_back", "rolled_back", "恢复中断，已恢复到覆盖前状态"
			active.Maintenance, active.JobID = false, ""
			_ = r.Coordinator.saveActive(active)
			return true, r.Coordinator.Store.Save(j)
		}
		if active.Generation != j.ExpectedGeneration || active.Maintenance {
			return false, errors.New("当前实例代次已经变化")
		}
		j.Status = "running"
		active.Maintenance, active.JobID = true, j.ID
		if err = r.Coordinator.saveActive(active); err != nil {
			return false, err
		}
		if err = r.phase(&j, "preflight_restore"); err != nil {
			return false, err
		}
		err = r.restoreCurrent(ctx, &j)
		if err != nil {
			j.Error = err.Error()
			if undo := r.compensateCurrent(context.WithoutCancel(ctx), &j); undo != nil {
				j.Status = "attention_required"
				j.Error = "恢复失败且补偿未完成，已保留维护现场"
				_ = r.Coordinator.Store.Save(j)
				return false, errors.New(j.Error)
			}
			j.Status, j.Phase = "failed", "failed"
			active.Maintenance, active.JobID = false, ""
			_ = r.Coordinator.saveActive(active)
			return true, r.Coordinator.Store.Save(j)
		}
		j.WritesMayHaveResumed = true
		return true, r.phase(&j, "verifying_startup")
	}
	return false, nil
}

func (r *CurrentRuntime) restoreCurrent(ctx context.Context, j *snapshots.ManagedJob) error {
	if r.spec.Environment != "dev" {
		return errors.New("当前外部基础设施的在线包尚不支持生产整站恢复，未覆盖任何数据")
	}
	source, err := r.Coordinator.SnapshotStore.LoadCoordinatorSource()
	if err != nil {
		return err
	}
	current, err := sourceConfig(source)
	if err != nil {
		return err
	}
	if err = r.checkConnection(source); err != nil {
		return err
	}
	if err = privateDirectory(r.work(j.ID)); err != nil {
		return err
	}
	staging := filepath.Join(r.work(j.ID), "snapshot")
	manifest, configuration, err := r.Coordinator.SnapshotStore.MaterializeForRecovery(privileged(ctx), j.SnapshotID, staging)
	if err != nil {
		return err
	}
	if manifest.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		return errors.New("快照程序平台与当前机器不一致，未覆盖当前实例")
	}
	var restored config.Config
	if err = yaml.Unmarshal(configuration["effective.yaml"], &restored); err != nil {
		return errors.New("快照运行配置无法读取")
	}
	if restored.Deployment.Env != current.Deployment.Env {
		return errors.New("快照与当前实例的部署环境不一致")
	}
	if current.Storage.DefaultDriver != "local" || len(current.Database.Replicas) > 0 {
		return errors.New("当前恢复只支持 PostgreSQL 主库和本地媒体")
	}
	if err = checkCurrentDatabase(ctx, source.DatabaseDSN); err != nil {
		return err
	}
	if err = checkCurrentRedis(ctx, current); err != nil {
		return err
	}
	if err = checkDump(ctx, filepath.Join(staging, "database/database.dump")); err != nil {
		return err
	}
	// 连接、端口、目录属于这一个安装；恢复业务配置和密钥，不创建或切换 DSN。
	restored.Database = current.Database
	restored.Server = current.Server
	restored.Storage = current.Storage
	restored.Cache = current.Cache
	restored.Queue = current.Queue
	restored.Event = current.Event
	restored.Recovery = current.Recovery
	restored.SystemSnapshots = current.SystemSnapshots
	restored.Plugin.InstalledDir = current.Plugin.InstalledDir
	restored.Plugin.RegistryFile = current.Plugin.RegistryFile
	journal, err := r.prepareCurrentFiles(ctx, j, source, staging, configuration, &restored)
	if err != nil {
		return err
	}
	if err = snapshots.WriteCoordinatorJSON(r.journalFile(j.ID), journal); err != nil {
		return err
	}
	if err = r.phase(j, "protecting_current"); err != nil {
		return err
	}
	// 所有业务连接已随 exec 退出。外部 PG/Redis 服务本身保持运行。
	redisFile := filepath.Join(r.work(j.ID), "redis-before.json")
	defer os.Remove(redisFile)
	if err = captureCurrentRedis(ctx, current, redisFile); err != nil {
		return err
	}
	if source.ConfigFiles == nil {
		source.ConfigFiles = map[string]string{}
	}
	source.ConfigFiles["recovery/current-redis.json"] = redisFile
	backend := &snapshots.PostgresBackend{DSN: source.DatabaseDSN, MediaRoot: source.MediaRoot}
	protection := snapshots.NewService(ctx, source, snapshots.Options{Coordinator: true, Backend: backend, DispatchCapture: func(context.Context, string) error { return nil }})
	defer protection.Close()
	point, err := protection.Start(privileged(ctx), uuid.NewSHA1(uuid.NameSpaceOID, []byte("protect-"+j.ID)).String())
	if err != nil {
		return err
	}
	j.RollbackSnapshotID = point.ID
	if err = r.Coordinator.Store.Save(*j); err != nil {
		return err
	}
	if err = protection.RunQueuedCapture(ctx, point.ID); err != nil {
		return errors.New("恢复前保护快照创建失败，未覆盖当前数据")
	}
	if _, err = protection.Protect(privileged(ctx), point.ID, true); err != nil {
		return err
	}
	if err = r.phase(j, "rollback_point_saved"); err != nil {
		return err
	}
	journal.DatabaseTouched = true
	if err = snapshots.WriteCoordinatorJSON(r.journalFile(j.ID), journal); err != nil {
		return err
	}
	if err = r.phase(j, "replacing_database"); err != nil {
		return err
	}
	if err = replaceCurrentDatabase(ctx, source.DatabaseDSN, filepath.Join(staging, "database/database.dump")); err != nil {
		return err
	}
	if err = r.installCurrentFiles(ctx, &journal); err != nil {
		return err
	}
	journal.RedisTouched = true
	if err = snapshots.WriteCoordinatorJSON(r.journalFile(j.ID), journal); err != nil {
		return err
	}
	if err = restoreCurrentRedis(ctx, current, configuration["recovery/current-redis.json"]); err != nil {
		return err
	}
	j.Checks = append(j.Checks, snapshots.RecoveryCheck{Name: "same_instance", Passed: true, Detail: "保留当前配置路径、DSN 和端口；数据库、媒体、配置和已登记程序已恢复，等待正常启动验证"})
	j.Checks = append(j.Checks, snapshots.RecoveryCheck{Name: "cache_scope", Passed: true, Detail: "缓存按当前安装与新代次重建；未清空共享缓存库，也未覆盖其他程序的缓存"})
	if len(configuration["recovery/current-redis.json"]) == 0 {
		j.Checks = append(j.Checks, snapshots.RecoveryCheck{Name: "online_queue_scope", Passed: true, Detail: "本地在线快照没有历史 Redis 队列；已清空本实例专用任务逻辑库，未重放快照之后的队列；共享缓存库不在整库恢复范围"})
	}
	return nil
}

func (r *CurrentRuntime) checkConnection(source snapshots.Config) error {
	sum := sha256.Sum256([]byte(source.DatabaseDSN))
	hash := fmt.Sprintf("%x", sum[:])
	if hash != r.spec.ConnectionHash {
		return errors.New("当前数据库连接身份已改变，拒绝按旧记录恢复")
	}
	return nil
}

func checkDump(ctx context.Context, file string) error {
	tool, err := exec.LookPath("pg_restore")
	if err != nil {
		return errors.New("缺少 pg_restore，请安装与数据库匹配的 PostgreSQL 客户端")
	}
	cmd := exec.CommandContext(ctx, tool, "--list", file)
	if err = cmd.Run(); err != nil {
		return errors.New("数据库归档无法读取，未覆盖当前数据库")
	}
	return nil
}

func currentAdmin(ctx context.Context, dsn string) (*pgx.Conn, *pgx.ConnConfig, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, nil, errors.New("当前数据库连接无效")
	}
	if cfg.Database == "" || cfg.Database == "postgres" || strings.HasPrefix(cfg.Database, "template") {
		return nil, nil, errors.New("拒绝覆盖 PostgreSQL 管理数据库")
	}
	admin := cfg.Copy()
	admin.Database = "postgres"
	c, err := pgx.ConnectConfig(ctx, admin)
	if err != nil {
		return nil, nil, errors.New("无法连接当前 PostgreSQL 管理库")
	}
	return c, cfg, nil
}
func checkCurrentDatabase(ctx context.Context, dsn string) error {
	c, cfg, err := currentAdmin(ctx, dsn)
	if err != nil {
		return err
	}
	defer c.Close(ctx)
	var others int
	var mayCreate bool
	if err = c.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=$1 AND pid<>pg_backend_pid()", cfg.Database).Scan(&others); err != nil {
		return errors.New("无法核对数据库维护连接")
	}
	if others != 0 {
		return errors.New("仍有其他程序连接当前数据库；未终止外部连接，也未覆盖数据")
	}
	if err = c.QueryRow(ctx, "SELECT rolsuper OR rolcreatedb FROM pg_roles WHERE rolname=current_user").Scan(&mayCreate); err != nil || !mayCreate {
		return errors.New("当前数据库账号没有重建自身数据库所需权限")
	}
	return nil
}
func replaceCurrentDatabase(ctx context.Context, dsn, file string) error {
	if err := checkCurrentDatabase(ctx, dsn); err != nil {
		return err
	}
	c, cfg, err := currentAdmin(ctx, dsn)
	if err != nil {
		return err
	}
	defer c.Close(ctx)
	var owner string
	err = c.QueryRow(ctx, "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname=$1", cfg.Database).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		owner = cfg.User
	} else if err != nil {
		return errors.New("无法读取当前数据库所有者")
	}
	name := pgx.Identifier{cfg.Database}.Sanitize()
	if _, err = c.Exec(ctx, "DROP DATABASE IF EXISTS "+name); err != nil {
		return errors.New("无法进入数据库恢复，未强制终止任何外部连接")
	}
	if _, err = c.Exec(ctx, "CREATE DATABASE "+name+" OWNER "+pgx.Identifier{owner}.Sanitize()+" TEMPLATE template0"); err != nil {
		return errors.New("当前数据库重建失败，已保留保护快照")
	}
	cmd := exec.CommandContext(ctx, "pg_restore", "--exit-on-error", "--single-transaction", "--dbname", dsn, file)
	if err = cmd.Run(); err != nil {
		return errors.New("数据库恢复失败，正在使用保护快照补偿")
	}
	return nil
}

func (r *CurrentRuntime) compensateCurrent(ctx context.Context, j *snapshots.ManagedJob) error {
	var journal currentJournal
	if err := snapshots.ReadCoordinatorJSON(r.journalFile(j.ID), &journal); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if journal.JobID != j.ID {
		return errors.New("恢复补偿日志身份不匹配")
	}
	if !journal.DatabaseTouched && j.RollbackSnapshotID != "" {
		if err := r.Coordinator.SnapshotStore.FailCoordinatorCapture(j.RollbackSnapshotID, "恢复在覆盖前中断，保护快照未完成"); err != nil {
			return err
		}
	}
	if journal.DatabaseTouched || journal.RedisTouched {
		if uuid.Validate(j.RollbackSnapshotID) != nil {
			return errors.New("保护快照缺失，禁止启动业务")
		}
		target := filepath.Join(r.work(j.ID), "undo")
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		_, configuration, err := r.Coordinator.SnapshotStore.MaterializeForRecovery(privileged(ctx), j.RollbackSnapshotID, target)
		if err != nil {
			return err
		}
		source, err := r.Coordinator.SnapshotStore.LoadCoordinatorSource()
		if err != nil {
			return err
		}
		if journal.DatabaseTouched {
			if err = replaceCurrentDatabase(ctx, source.DatabaseDSN, filepath.Join(target, "database/database.dump")); err != nil {
				return err
			}
		}
		cfg, err := sourceConfig(source)
		if err != nil {
			return err
		}
		if journal.RedisTouched {
			if err = restoreCurrentRedis(ctx, cfg, configuration["recovery/current-redis.json"]); err != nil {
				return err
			}
		}
	}
	return r.revertCurrentFiles(journal)
}

// AwaitingStartup 仅允许本次 exec 进入正常启动；冷启动遇到未完成现场会先补偿／保持维护。
func (r *CurrentRuntime) AwaitingStartup() bool {
	id := os.Getenv("POWERX_RECOVERY_BOOT_JOB")
	if uuid.Validate(id) != nil {
		return false
	}
	j, err := r.Coordinator.Store.Get(id)
	return err == nil && j.Phase == "verifying_startup" && j.Status == "running"
}
func (r *CurrentRuntime) FinishStartup() error {
	if !r.AwaitingStartup() {
		return nil
	}
	var expected config.Config
	raw, err := os.ReadFile(r.spec.ConfigFile)
	if err != nil || yaml.Unmarshal(raw, &expected) != nil {
		return errors.New("恢复后的正式配置不可读取")
	}
	loaded := config.GetGlobalConfig()
	if loaded.Auth.JWTSecret != expected.Auth.JWTSecret || loaded.Auth.Issuer != expected.Auth.Issuer {
		return errors.New("环境变量覆盖了恢复后的登录配置，已阻止完成确认")
	}
	id := os.Getenv("POWERX_RECOVERY_BOOT_JOB")
	j, err := r.Coordinator.Store.Get(id)
	if err != nil {
		return err
	}
	a, err := r.Coordinator.Active()
	if err != nil {
		return err
	}
	if j.Kind == "resume" {
		if parent, e := r.Coordinator.Store.Get(j.CandidateID); e == nil && parent.Status == "attention_required" {
			parent.Status = "failed"
			_ = r.Coordinator.Store.Save(parent)
		}
	}
	a.Maintenance, a.JobID = false, ""
	a.Generation++
	if err = r.Coordinator.saveActive(a); err != nil {
		return err
	}
	j.Status, j.Phase, j.BusinessVerified = "complete", "complete", true
	if err = r.Coordinator.Store.Save(j); err != nil {
		return err
	}
	var journal currentJournal
	if snapshots.ReadCoordinatorJSON(r.journalFile(j.ID), &journal) == nil {
		for _, f := range journal.Files {
			_ = os.RemoveAll(f.Backup)
			_ = os.RemoveAll(f.Prepared)
		}
	}
	_ = os.RemoveAll(r.work(j.ID))
	_ = os.Unsetenv("POWERX_RECOVERY_BOOT_JOB")
	return nil
}
