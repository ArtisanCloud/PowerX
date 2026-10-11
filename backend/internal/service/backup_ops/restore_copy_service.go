package backup_ops

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	pxcrypto "github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sys/unix"
	"gorm.io/gorm"
)

type RestoreCopyOptions struct{ Directory string }

// RestoreCopyService 管理独立恢复库，不提供主实例连接切换。
type RestoreCopyService struct {
	db        *gorm.DB
	cfg       *pgx.ConnConfig
	sourceKey string
	store     *restoreCopyStore
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	workers   sync.WaitGroup
	closeOnce sync.Once
	lockFile  *os.File
	initErr   error
}

func NewRestoreCopyService(ctx context.Context, db *gorm.DB, opts RestoreCopyOptions) *RestoreCopyService {
	if ctx == nil {
		ctx = context.Background()
	}
	run, cancel := context.WithCancel(ctx)
	s := &RestoreCopyService{db: db, ctx: run, cancel: cancel}
	if db == nil || sourceDSN(db) == "" {
		s.initErr = errors.New("restore source unavailable")
		return s
	}
	cfg, err := pgx.ParseConfig(sourceDSN(db))
	if err != nil {
		s.initErr = err
		return s
	}
	s.cfg = cfg
	s.sourceKey = restoreSourceKey(cfg.Host, cfg.Port, cfg.Database)
	s.store, err = newRestoreCopyStore(opts.Directory, s.sourceKey)
	if err != nil {
		s.initErr = err
		return s
	}
	s.lockFile, err = os.OpenFile(filepath.Join(s.store.root, ".coordinator.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err == nil {
		err = unix.Flock(int(s.lockFile.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	}
	if err != nil {
		s.initErr = err
		if s.lockFile != nil {
			s.lockFile.Close()
		}
		return s
	}
	// 同一清单目录仅一个协调器；接管已退出进程的任务时不重新导入或删除数据库。
	rows, err := s.store.list()
	if err != nil {
		s.initErr = err
	} else {
		for _, row := range rows {
			if row.Status == "queued" || row.Status == "running" {
				s.revokeRestoreOwner(row)
				row.OwnerPassword = nil
				row.Status = "failed"
				row.Phase = "interrupted"
				row.ErrorCode = "RESTORE_INTERRUPTED"
				row.ErrorMessage = "恢复进程已中断；已创建的新库保留供检查"
				if err = s.store.save(row); err != nil {
					s.initErr = err
					break
				}
			}
		}
	}
	if s.initErr == nil {
		s.workers.Add(1)
		go s.expireReaderAccess()
	}
	context.AfterFunc(run, s.closeResources)
	return s
}

func (s *RestoreCopyService) Close() { s.mu.Lock(); s.cancel(); s.mu.Unlock(); s.closeResources() }
func (s *RestoreCopyService) closeResources() {
	s.mu.Lock()
	s.mu.Unlock()
	s.workers.Wait()
	s.closeOnce.Do(func() {
		if s.lockFile != nil {
			unix.Flock(int(s.lockFile.Fd()), unix.LOCK_UN)
			s.lockFile.Close()
		}
	})
}

func (s *RestoreCopyService) authorize(ctx context.Context) error {
	if !reqctx.IsRoot(ctx) {
		return restoreFailure(403, "RESTORE_ROOT_REQUIRED", "完整数据库恢复仅允许平台 Root 管理员")
	}
	if c := reqctx.GetClaims(ctx); c != nil {
		if c.Issuer == "powerx-sts" {
			return restoreFailure(403, "RESTORE_USER_SESSION_REQUIRED", "恢复需要管理员用户会话")
		}
		for _, plane := range c.Platforms {
			if plane == "api_key" {
				return restoreFailure(403, "RESTORE_USER_SESSION_REQUIRED", "API Key 不能执行整库恢复")
			}
		}
	}
	if s == nil || s.initErr != nil || s.cfg == nil || s.store == nil {
		return restoreFailure(503, "RESTORE_SERVICE_UNAVAILABLE", "恢复协调器不可用，检查数据库、存储权限或重复运行的实例")
	}
	if s.ctx.Err() != nil {
		return restoreFailure(503, "RESTORE_SERVICE_STOPPED", "恢复服务已停止")
	}
	return nil
}

func (s *RestoreCopyService) connect(ctx context.Context, database, user, password string) (*pgx.Conn, error) {
	cfg := s.cfg.Copy()
	cfg.Database = database
	cfg.User = user
	cfg.Password = password
	cfg.ConnectTimeout = 10 * time.Second
	return pgx.ConnectConfig(ctx, cfg)
}

type CreateRestoreCopy struct {
	RestoreSource
	IdempotencyKey   string
	AllowOlderServer bool
	Operator         string
	TraceID          string
}

func (s *RestoreCopyService) Create(ctx context.Context, in CreateRestoreCopy) (RestoreCopy, error) {
	if err := s.authorize(ctx); err != nil {
		return RestoreCopy{}, err
	}
	if !validCopyID(in.IdempotencyKey) {
		return RestoreCopy{}, restoreFailure(400, "RESTORE_IDEMPOTENCY_REQUIRED", "恢复请求需要 UUID Idempotency-Key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return RestoreCopy{}, restoreFailure(503, "RESTORE_SERVICE_STOPPED", "恢复服务已停止")
	}
	sum := sha256.Sum256([]byte(in.IdempotencyKey))
	key := hex.EncodeToString(sum[:])
	rows, err := s.store.list()
	if err != nil {
		return RestoreCopy{}, err
	}
	for _, row := range rows {
		if row.IdempotencyHash == key {
			if row.SourceJobID != in.SourceJobID || row.ImportID != in.ImportID || row.AllowOlderServer != in.AllowOlderServer {
				return RestoreCopy{}, restoreFailure(409, "RESTORE_IDEMPOTENCY_CONFLICT", "同一请求标识不能用于另一份备份或参数")
			}
			return row.RestoreCopy, nil
		}
	}
	for _, row := range rows {
		if row.Status == "queued" || row.Status == "running" {
			return RestoreCopy{}, restoreFailure(409, "RESTORE_COPY_BUSY", "当前实例已有恢复任务，请等待完成")
		}
	}
	plan, err := s.Plan(ctx, in.RestoreSource)
	if err != nil {
		return RestoreCopy{}, err
	}
	if !plan.Ready {
		return RestoreCopy{}, restoreFailure(503, "RESTORE_DEPENDENCY_UNAVAILABLE", strings.Join(plan.Problems, "；"))
	}
	if plan.RequiresCompatibilityConsent && !in.AllowOlderServer {
		return RestoreCopy{}, restoreFailure(409, "RESTORE_VERSION_CONFIRMATION_REQUIRED", "目标 PostgreSQL 版本较低，需要明确确认兼容恢复尝试")
	}
	archive, err := s.archive(ctx, in.RestoreSource)
	if err != nil {
		return RestoreCopy{}, err
	}
	unlock := func() {}
	if archive.PolicyID > 0 {
		unlock, err = lockBackupPolicy(ctx, s.db, archive.PolicyID)
		if err != nil {
			return RestoreCopy{}, restoreFailure(409, "RESTORE_SOURCE_BUSY", "备份正在使用或清理，请稍后重试")
		}
	}
	id := uuid.NewString()
	token := strings.ReplaceAll(id, "-", "")
	password, err := restoreRandomPassword()
	if err != nil {
		unlock()
		return RestoreCopy{}, err
	}
	wrapped, err := pxcrypto.WrapPrivateKey([]byte(password))
	if err != nil {
		unlock()
		return RestoreCopy{}, restoreFailure(503, "RESTORE_SECRET_UNAVAILABLE", "副本凭据加密密钥不可用")
	}
	row := copyLedger{RestoreCopy: RestoreCopy{ID: id, SourceJobID: in.SourceJobID, ImportID: in.ImportID, SourceName: archive.Name, SourceSHA256: archive.Checksum, Host: s.cfg.Host, Port: s.cfg.Port, CurrentDatabase: s.cfg.Database, Database: "powerx_restore_copy_" + token, Status: "queued", Phase: "queued", CreatedAt: time.Now().UTC(), Operator: normalizeOperator(in.Operator), TraceID: in.TraceID, Warnings: []string{}}, SourceKey: s.sourceKey, IdempotencyHash: key, OwnerRole: "pxrc_owner_" + token, OwnerPassword: wrapped, ReaderRole: "pxrc_read_" + token}
	if plan.RequiresCompatibilityConsent {
		row.Warnings = append(row.Warnings, "归档来自较新 PostgreSQL，已确认尝试兼容恢复；仍需业务和版本核验")
	}
	row.AllowOlderServer = in.AllowOlderServer
	row.UpdatedAt = row.CreatedAt
	if err = s.store.save(row); err != nil {
		unlock()
		return RestoreCopy{}, err
	}
	s.workers.Add(1)
	go func() { defer s.workers.Done(); defer unlock(); s.execute(row, archive) }()
	return row.RestoreCopy, nil
}

func restoreRandomPassword() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}

func (s *RestoreCopyService) List(ctx context.Context) ([]RestoreCopy, error) {
	if err := s.authorize(ctx); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.store.list()
	if err != nil {
		return nil, err
	}
	out := []RestoreCopy{}
	for _, row := range rows {
		if row.SourceKey != s.sourceKey {
			return nil, restoreFailure(503, "RESTORE_SOURCE_CHANGED", "恢复清单与当前实例不匹配")
		}
		out = append(out, row.RestoreCopy)
	}
	return out, nil
}

func (s *RestoreCopyService) Get(ctx context.Context, id string) (RestoreCopy, error) {
	if err := s.authorize(ctx); err != nil {
		return RestoreCopy{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.managed(id)
	return row.RestoreCopy, err
}

func (s *RestoreCopyService) managed(id string) (copyLedger, error) {
	row, err := s.store.read(id)
	if err != nil {
		return row, err
	}
	token := strings.ReplaceAll(id, "-", "")
	if row.SourceKey != s.sourceKey || row.Database != "powerx_restore_copy_"+token || row.OwnerRole != "pxrc_owner_"+token || row.ReaderRole != "pxrc_read_"+token || row.Database == s.cfg.Database {
		return row, restoreFailure(409, "RESTORE_TARGET_INVALID", "恢复目标身份不匹配，拒绝操作")
	}
	return row, nil
}

func (s *RestoreCopyService) persist(row copyLedger) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.save(row)
}

func (s *RestoreCopyService) phase(row *copyLedger, phase string) error {
	row.Status = "running"
	row.Phase = phase
	return s.persist(*row)
}

func (s *RestoreCopyService) databaseIdentity(ctx context.Context, admin *pgx.Conn, row copyLedger) error {
	if row.DatabaseOID == 0 || row.Database == s.cfg.Database {
		return restoreFailure(409, "RESTORE_TARGET_INVALID", "当前库或未登记目标不能操作")
	}
	var oid uint32
	var owner string
	var ownerOID uint32
	err := admin.QueryRow(ctx, "SELECT d.oid,r.rolname,r.oid FROM pg_database d JOIN pg_roles r ON r.oid=d.datdba WHERE d.datname=$1", row.Database).Scan(&oid, &owner, &ownerOID)
	if errors.Is(err, pgx.ErrNoRows) {
		return restoreFailure(404, "RESTORE_DATABASE_NOT_FOUND", "恢复库已不存在")
	}
	if err != nil {
		return err
	}
	if oid != row.DatabaseOID || owner != row.OwnerRole || ownerOID != row.OwnerRoleOID {
		return restoreFailure(409, "RESTORE_TARGET_CHANGED", "恢复库 OID 或所有者改变，拒绝操作")
	}
	return nil
}

func (s *RestoreCopyService) Delete(ctx context.Context, id, confirmation string) error {
	if err := s.authorize(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.managed(id)
	if err != nil {
		return err
	}
	if row.Status == "deleted" {
		return nil
	}
	if row.Status == "queued" || row.Status == "running" {
		return restoreFailure(409, "RESTORE_COPY_RUNNING", "正在恢复，不能删除")
	}
	if confirmation != row.Database {
		return restoreFailure(400, "RESTORE_CONFIRMATION_REQUIRED", "请输入完整新库名确认删除副本")
	}
	admin, err := s.connect(ctx, s.cfg.Database, s.cfg.User, s.cfg.Password)
	if err != nil {
		return err
	}
	defer admin.Close(ctx)
	if row.DatabaseOID > 0 {
		// 库已删除而角色清理中断时可以重试；同名新库仍必须拒绝。
		var exists bool
		if err = admin.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", row.Database).Scan(&exists); err != nil {
			return err
		}
		if exists {
			if err = s.databaseIdentity(ctx, admin, row); err != nil {
				return err
			}
			var active int
			err = admin.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=$1", row.Database).Scan(&active)
			if err != nil {
				return err
			}
			if active > 0 {
				return restoreFailure(409, "RESTORE_COPY_IN_USE", "恢复库仍有连接，请先关闭连接；不会强制终止")
			}
			if _, err = admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{row.Database}.Sanitize()); err != nil {
				return restoreFailure(409, "RESTORE_COPY_DELETE_FAILED", "删除副本失败，数据库可能仍被连接")
			}
		}
	} else {
		var exists bool
		if err = admin.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)", row.Database).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return restoreFailure(409, "RESTORE_TARGET_UNTRACKED", "新库登记不完整，拒绝自动删除，请人工核对")
		}
	}
	for _, role := range []struct {
		name string
		oid  uint32
	}{{row.ReaderRole, row.ReaderRoleOID}, {row.OwnerRole, row.OwnerRoleOID}} {
		if role.oid == 0 {
			continue
		}
		var actual uint32
		err = admin.QueryRow(ctx, "SELECT oid FROM pg_roles WHERE rolname=$1", role.name).Scan(&actual)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if actual != role.oid {
			return restoreFailure(409, "RESTORE_ROLE_CHANGED", "副本角色身份已改变，拒绝删除")
		}
		if _, err = admin.Exec(ctx, "DROP ROLE "+pgx.Identifier{role.name}.Sanitize()); err != nil {
			return restoreFailure(409, "RESTORE_ROLE_IN_USE", "副本已删除，但专用角色存在其他依赖，需要人工检查")
		}
	}
	row.Status = "deleted"
	row.Phase = "deleted"
	row.OwnerPassword = nil
	if err = s.store.save(row); err != nil {
		return err
	}
	// 删除最后一个引用副本后，同步清理本次上传副本；已登记备份始终交给原备份保留策略管理。
	rows, err := s.store.list()
	if err == nil && row.ImportID != "" {
		used := false
		for _, other := range rows {
			if other.ImportID == row.ImportID && other.Status != "deleted" {
				used = true
			}
		}
		if !used {
			s.removeImportFiles(row.ImportID)
		}
	}
	return nil
}

type RestoreCopyConnection struct {
	Host      string    `json:"host"`
	Port      uint16    `json:"port"`
	Database  string    `json:"database"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	ExpiresAt time.Time `json:"expires_at"`
	ReadOnly  bool      `json:"read_only"`
	SSLMode   string    `json:"ssl_mode"`
}

func (s *RestoreCopyService) Connection(ctx context.Context, id string) (RestoreCopyConnection, error) {
	if err := s.authorize(ctx); err != nil {
		return RestoreCopyConnection{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.managed(id)
	if err != nil {
		return RestoreCopyConnection{}, err
	}
	if row.Status != "success" {
		return RestoreCopyConnection{}, restoreFailure(409, "RESTORE_COPY_NOT_READY", "只有完整恢复成功的副本可生成只读连接")
	}
	admin, err := s.connect(ctx, s.cfg.Database, s.cfg.User, s.cfg.Password)
	if err != nil {
		return RestoreCopyConnection{}, err
	}
	defer admin.Close(ctx)
	if err = s.databaseIdentity(ctx, admin, row); err != nil {
		return RestoreCopyConnection{}, err
	}
	if err = restoreRoleIdentity(ctx, admin, row.ReaderRole, row.ReaderRoleOID); err != nil {
		return RestoreCopyConnection{}, err
	}
	password, err := restoreRandomPassword()
	if err != nil {
		return RestoreCopyConnection{}, err
	}
	verifier, err := restoreSCRAM(password)
	if err != nil {
		return RestoreCopyConnection{}, err
	}
	expires := time.Now().UTC().Add(24 * time.Hour)
	query := "ALTER ROLE " + pgx.Identifier{row.ReaderRole}.Sanitize() + " LOGIN PASSWORD " + restoreSQLLiteral(verifier) + " VALID UNTIL " + restoreSQLLiteral(expires.Format(time.RFC3339))
	if _, err = admin.Exec(ctx, query); err != nil {
		return RestoreCopyConnection{}, restoreFailure(503, "RESTORE_ACCESS_FAILED", "生成副本只读凭据失败")
	}
	row.ReaderExpiresAt = expires
	if err = s.store.save(row); err != nil {
		_, _ = admin.Exec(ctx, "ALTER ROLE "+pgx.Identifier{row.ReaderRole}.Sanitize()+" NOLOGIN PASSWORD NULL")
		return RestoreCopyConnection{}, restoreFailure(503, "RESTORE_ACCESS_FAILED", "无法登记凭据有效期，已撤销本次连接")
	}
	sslMode := "prefer"
	if env, err := postgresEnvironment(sourceDSN(s.db)); err == nil {
		for _, value := range env {
			if strings.HasPrefix(value, "PGSSLMODE=") {
				sslMode = strings.TrimPrefix(value, "PGSSLMODE=")
			}
		}
	}
	return RestoreCopyConnection{Host: s.cfg.Host, Port: s.cfg.Port, Database: row.Database, Username: row.ReaderRole, Password: password, ExpiresAt: expires, ReadOnly: true, SSLMode: sslMode}, nil
}

func restoreRoleIdentity(ctx context.Context, conn *pgx.Conn, name string, expected uint32) error {
	var oid uint32
	if err := conn.QueryRow(ctx, "SELECT oid FROM pg_roles WHERE rolname=$1", name).Scan(&oid); err != nil || expected == 0 || oid != expected {
		return restoreFailure(409, "RESTORE_ROLE_CHANGED", "副本角色身份已改变，拒绝操作")
	}
	return nil
}

// 密码过期之外再撤销 LOGIN，覆盖本地 trust 认证；已有只读连接不强制终止。
func (s *RestoreCopyService) expireReaderAccess() {
	defer s.workers.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		s.revokeExpiredReaders()
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *RestoreCopyService) revokeExpiredReaders() {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.store.list()
	if err != nil {
		return
	}
	for _, row := range rows {
		if row.ReaderExpiresAt.IsZero() || time.Now().Before(row.ReaderExpiresAt) || row.Status == "deleted" {
			continue
		}
		if _, err = s.managed(row.ID); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
		admin, connectErr := s.connect(ctx, s.cfg.Database, s.cfg.User, s.cfg.Password)
		if connectErr == nil {
			if err = restoreRoleIdentity(ctx, admin, row.ReaderRole, row.ReaderRoleOID); err == nil {
				_, err = admin.Exec(ctx, "ALTER ROLE "+pgx.Identifier{row.ReaderRole}.Sanitize()+" NOLOGIN PASSWORD NULL")
				if err == nil {
					row.ReaderExpiresAt = time.Time{}
					_ = s.store.save(row)
				}
			}
			admin.Close(ctx)
		}
		cancel()
	}
	// 未形成任务的上传归档保留 24 小时，避免取消向导后占用永久磁盘。
	used := map[string]bool{}
	for _, row := range rows {
		if row.Status != "deleted" {
			used[row.ImportID] = true
		}
	}
	entries, _ := os.ReadDir(filepath.Join(s.store.root, "imports"))
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		if entry.Name() != id+".json" || !validCopyID(id) || used[id] {
			continue
		}
		path := filepath.Join(s.store.root, "imports", entry.Name())
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var metadata importLedger
		if json.Unmarshal(raw, &metadata) == nil && metadata.ID == id && metadata.Binding == s.sourceKey && !metadata.CreatedAt.IsZero() && time.Since(metadata.CreatedAt) > 24*time.Hour {
			s.removeImportFiles(id)
		}
	}
}

func (s *RestoreCopyService) removeImportFiles(id string) {
	if !validCopyID(id) {
		return
	}
	for _, suffix := range []string{".dump", ".json"} {
		path := filepath.Join(s.store.root, "imports", id+suffix)
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			_ = os.Remove(path)
		}
	}
}

func restoreSQLLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }

func (s *RestoreCopyService) fail(row copyLedger, err error) {
	s.revokeRestoreOwner(row)
	row.OwnerPassword = nil
	phase := row.Phase
	row.Status = "failed"
	row.Phase = "failed"
	row.ErrorCode = "RESTORE_EXECUTION_FAILED"
	row.ErrorMessage = fmt.Sprintf("恢复在 %s 阶段失败；新库保留供检查：%s", phase, err.Error())
	if s.ctx.Err() != nil {
		row.ErrorCode = "RESTORE_INTERRUPTED"
		row.ErrorMessage = "服务停止，恢复已中断；已创建的新库保留供检查"
	}
	_ = s.persist(row)
}

func (s *RestoreCopyService) revokeRestoreOwner(row copyLedger) {
	if row.OwnerRoleOID == 0 {
		return
	}
	if _, err := s.managed(row.ID); err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := s.connect(ctx, s.cfg.Database, s.cfg.User, s.cfg.Password)
	if err != nil {
		return
	}
	defer conn.Close(ctx)
	if restoreRoleIdentity(ctx, conn, row.OwnerRole, row.OwnerRoleOID) == nil {
		_, _ = conn.Exec(ctx, "ALTER ROLE "+pgx.Identifier{row.OwnerRole}.Sanitize()+" NOLOGIN PASSWORD NULL")
	}
}
