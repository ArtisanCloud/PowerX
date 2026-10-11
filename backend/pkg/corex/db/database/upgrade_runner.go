package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const UpgradeResultSchema = "powerx.migration-result/v1"

// UpgradeRun 记录每次执行，包括失败和被中断的执行；不包含配置或 SQL 错误正文。
type UpgradeRun struct {
	RunUUID            uuid.UUID  `gorm:"type:uuid;primaryKey" json:"run_uuid"`
	SourceSHA          string     `gorm:"size:40;not null" json:"source_sha"`
	ReleaseID          string     `gorm:"size:128;not null" json:"target_release"`
	MigrationVersion   string     `gorm:"size:64;not null;default:''" json:"migration_version"`
	PlanHash           string     `gorm:"size:64;not null" json:"plan_hash"`
	Status             string     `gorm:"size:32;not null" json:"status"`
	MigrationID        string     `gorm:"size:128" json:"migration_id"`
	ErrorCode          string     `gorm:"size:80" json:"error_code"`
	DatabaseErrorCode  string     `gorm:"size:5" json:"database_error_code"`
	DatabaseConstraint string     `gorm:"size:128" json:"database_constraint"`
	Partial            bool       `json:"partial"`
	Retryable          bool       `json:"retryable"`
	RequiresRestore    bool       `json:"requires_restore"`
	StartedAt          time.Time  `json:"started_at"`
	FinishedAt         *time.Time `json:"finished_at"`
}

func (UpgradeRun) TableName() string { return coremodel.PowerXSchema + ".database_upgrade_runs" }

// UpgradeStep 的描述及校验和随镜像发布，调用方不能传入 SQL 或替换执行器。
type UpgradeStep struct {
	ID            string                                `json:"id"`
	Checksum      string                                `json:"checksum"`
	DependsOn     []string                              `json:"depends_on"`
	Transactional bool                                  `json:"transactional"`
	Recovery      string                                `json:"recovery"`
	Apply         func(context.Context, *gorm.DB) error `json:"-"`
	Verify        func(context.Context, *gorm.DB) error `json:"-"`
}

type UpgradeBlocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type UpgradeResult struct {
	SchemaVersion           string           `json:"schema_version"`
	Operation               string           `json:"operation"`
	RunUUID                 *uuid.UUID       `json:"run_uuid"`
	SourceSHA               string           `json:"source_sha"`
	SourceRelease           string           `json:"source_release"`
	CurrentRelease          string           `json:"current_release"`
	CurrentMigrationVersion string           `json:"current_migration_version"`
	TargetRelease           string           `json:"target_release"`
	MigrationVersion        string           `json:"migration_version"`
	Database                string           `json:"database"`
	Schema                  string           `json:"schema"`
	PlanHash                string           `json:"plan_hash"`
	Status                  string           `json:"status"`
	Applied                 []string         `json:"applied"`
	Skipped                 []string         `json:"skipped"`
	Pending                 []UpgradeStep    `json:"pending"`
	Failed                  []string         `json:"failed"`
	Blockers                []UpgradeBlocker `json:"blockers"`
	ConfigurationChanges    []string         `json:"configuration_changes"`
	IncompleteRuns          []UpgradeRun     `json:"incomplete_runs"`
	Partial                 bool             `json:"partial"`
	Retryable               bool             `json:"retryable"`
	RequiresRestore         bool             `json:"requires_restore"`
	ErrorCode               string           `json:"error_code"`
	DatabaseErrorCode       string           `json:"database_error_code"`
	DatabaseConstraint      string           `json:"database_constraint"`
	Message                 string           `json:"message"`
	MaintenanceRequired     bool             `json:"maintenance_required"`
	Rollback                string           `json:"rollback"`
}

// UpgradeRunner 只支持 PostgreSQL 离线升级；锁固定绑定目标库与 schema。
type UpgradeRunner struct {
	DB               *gorm.DB
	ReleaseID        string
	MigrationVersion string
	SourceSHA        string
	Steps            []UpgradeStep
	// Fingerprint 包含完整配置和 schema/关键数据的摘要，不向结果输出内容。
	Fingerprint string
	Preflight   func(context.Context, *gorm.DB) ([]UpgradeBlocker, string, error)
}

func NewUpgradeResult(operation, target string) *UpgradeResult {
	return &UpgradeResult{SchemaVersion: UpgradeResultSchema, Operation: operation,
		TargetRelease: target, Schema: coremodel.PowerXSchema, Status: "ready",
		CurrentRelease: "unrecorded", CurrentMigrationVersion: "legacy-unversioned",
		Applied: []string{}, Skipped: []string{}, Pending: []UpgradeStep{}, Failed: []string{},
		Blockers: []UpgradeBlocker{}, ConfigurationChanges: []string{}, IncompleteRuns: []UpgradeRun{},
		MaintenanceRequired: true, Rollback: "restore_required"}
}

func (r *UpgradeResult) Block(code, message string) {
	r.Status = "blocked"
	r.ErrorCode = code
	r.Message = message
	r.Blockers = append(r.Blockers, UpgradeBlocker{code, message})
}

// Plan 不执行 DDL/DML，也不创建履历表。
func (r *UpgradeRunner) Plan(ctx context.Context, operation string) (*UpgradeResult, error) {
	out := NewUpgradeResult(operation, r.ReleaseID)
	out.SourceSHA = r.SourceSHA
	if r.SourceSHA != "" {
		out.SourceRelease = "sha-" + r.SourceSHA
	}
	out.MigrationVersion = r.MigrationVersion
	db := r.DB.WithContext(ctx)
	if db.Dialector.Name() != "postgres" {
		out.Block("UPGRADE_DATABASE_UNSUPPORTED", "Only PostgreSQL offline upgrades are supported")
		return out, nil
	}
	if err := db.Raw("SELECT current_database()").Scan(&out.Database).Error; err != nil {
		return out, err
	}
	if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(coremodel.PowerXSchema) {
		return out, fmt.Errorf("invalid schema")
	}
	seen := map[string]bool{}
	for _, step := range r.Steps {
		if seen[step.ID] || step.ID == "" || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(step.Checksum) || step.Apply == nil {
			return out, fmt.Errorf("invalid registry")
		}
		for _, dep := range step.DependsOn {
			if !seen[dep] {
				return out, fmt.Errorf("invalid dependency")
			}
		}
		seen[step.ID] = true
	}
	type state struct {
		MigrationID string
		Checksum    string
		ReleaseID   string
	}
	records := []state{}
	journal := coremodel.PowerXSchema + ".database_migration_records"
	if db.Migrator().HasTable(journal) {
		// 旧版本只有 migration_id / applied_at；按真实列读取，不能触发 AutoMigrate。
		columns := "migration_id"
		if db.Migrator().HasColumn(journal, "checksum") {
			columns += ", checksum, release_id"
		}
		if err := db.Table(journal).Select(columns).Order("migration_id").Scan(&records).Error; err != nil {
			return out, err
		}
	}
	completed := map[string]state{}
	for _, v := range records {
		if strings.HasPrefix(v.MigrationID, "20261011_") && (len(v.Checksum) != 64 || !strings.HasSuffix(v.MigrationID, "_"+v.Checksum)) {
			out.Block("UPGRADE_MIGRATION_CHECKSUM_MISMATCH", "A content-addressed migration journal entry is inconsistent")
		}
		completed[v.MigrationID] = v
	}
	for _, step := range r.Steps {
		if prev, ok := completed[step.ID]; ok {
			if prev.Checksum != step.Checksum {
				out.Block("UPGRADE_MIGRATION_CHECKSUM_MISMATCH", "An applied migration differs from the target registry")
			}
			out.Skipped = append(out.Skipped, step.ID)
		} else {
			out.Pending = append(out.Pending, step)
		}
	}
	if db.Migrator().HasTable(&UpgradeRun{}) {
		var latest []UpgradeRun
		if err := db.Where("status = ?", "succeeded").Order("started_at DESC").Limit(1).Find(&latest).Error; err != nil {
			return out, err
		}
		if len(latest) > 0 {
			out.CurrentRelease = latest[0].ReleaseID
			out.CurrentMigrationVersion = latest[0].MigrationVersion
		}
		if err := db.Where("status IN ?", []string{"running", "failed", "interrupted"}).Order("started_at").Find(&out.IncompleteRuns).Error; err != nil {
			return out, err
		}
		for _, run := range out.IncompleteRuns {
			if run.RequiresRestore || (run.Status == "running" && run.MigrationID != "" && !transactionalStep(r.Steps, run.MigrationID)) {
				out.RequiresRestore = true
				out.Block("UPGRADE_RECOVERY_REQUIRED", "Restore the matching pre-upgrade backup before retrying a nontransactional step")
			}
		}
	}
	var observed string
	if r.Preflight != nil {
		blockers, snapshot, err := r.Preflight(ctx, db)
		if err != nil {
			return out, err
		}
		observed = snapshot
		for _, b := range blockers {
			out.Block(b.Code, b.Message)
		}
	}
	// 同时冻结配置、schema/数据条件和履历。预览后任何漂移都必须重新 plan。
	payload, err := json.Marshal(struct {
		Target, Source, Version, Database, Schema, Config, Observed string
		Steps                                                       []UpgradeStep
		Records                                                     []state
		Runs                                                        []UpgradeRun
	}{
		r.ReleaseID, r.SourceSHA, r.MigrationVersion, out.Database, out.Schema, r.Fingerprint, observed, r.Steps, records, out.IncompleteRuns})
	if err != nil {
		return out, err
	}
	sum := sha256.Sum256(payload)
	out.PlanHash = hex.EncodeToString(sum[:])
	if len(out.Pending) == 0 && len(out.Blockers) == 0 {
		out.Status = "up_to_date"
	}
	return out, nil
}

func transactionalStep(steps []UpgradeStep, id string) bool {
	for _, s := range steps {
		if s.ID == id {
			return s.Transactional
		}
	}
	return false
}

// Apply 用专用连接持有 advisory lock；每个事务内同时提交变更和成功记录。
func (r *UpgradeRunner) Apply(ctx context.Context, expected string, lockTimeout time.Duration) (*UpgradeResult, error) {
	out := NewUpgradeResult("apply", r.ReleaseID)
	if expected == "" {
		out.Block("UPGRADE_EXPECTED_PLAN_REQUIRED", "Run plan and submit its exact hash")
		return out, nil
	}
	if lockTimeout < 0 || lockTimeout > 5*time.Minute {
		out.Block("UPGRADE_LOCK_TIMEOUT_INVALID", "Lock timeout must be between 0 and 300 seconds")
		return out, nil
	}
	err := WithDatabaseMigrationLock(ctx, r.DB, lockTimeout, func(conn *gorm.DB) error {
		lockedRunner := *r
		lockedRunner.DB = conn
		plan, err := lockedRunner.Plan(ctx, "apply")
		out = plan
		if err != nil {
			return err
		}
		if len(out.Blockers) != 0 {
			return nil
		}
		if out.PlanHash != expected {
			out.Block("UPGRADE_PLAN_CHANGED", "Database or configuration changed after preview; run plan again")
			return nil
		}
		if err := MigrateUpgradeJournal(conn); err != nil {
			return err
		}
		// 持锁证明旧 running 不再有执行者；事务步骤可重试，非事务已在 Plan 阻断。
		if err := conn.Model(&UpgradeRun{}).Where("status = ?", "running").Updates(map[string]any{"status": "interrupted", "error_code": "UPGRADE_INTERRUPTED", "retryable": true, "finished_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		run := UpgradeRun{RunUUID: uuid.New(), SourceSHA: r.SourceSHA, ReleaseID: r.ReleaseID, MigrationVersion: r.MigrationVersion, PlanHash: expected, Status: "running", StartedAt: time.Now().UTC()}
		out.RunUUID = &run.RunUUID
		if err := conn.Create(&run).Error; err != nil {
			return err
		}
		for _, step := range out.Pending {
			if err := conn.Model(&run).Update("migration_id", step.ID).Error; err != nil {
				return err
			}
			execute := func(tx *gorm.DB) (err error) {
				defer func() {
					if recover() != nil {
						err = fmt.Errorf("UPGRADE_STEP_PANIC")
					}
				}()
				tx = tx.Session(&gorm.Session{NewDB: true})
				if err := tx.Exec("SELECT set_config('powerx.upgrade_step', ?, ?)", step.ID, step.Transactional).Error; err != nil {
					return err
				}
				if !step.Transactional {
					defer func() {
						cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						_ = tx.WithContext(cleanup).Exec("SELECT set_config('powerx.upgrade_step', '', false)").Error
					}()
				}
				if err := step.Apply(ctx, tx); err != nil {
					return err
				}
				if step.Verify != nil {
					if err := step.Verify(ctx, tx); err != nil {
						return err
					}
				}
				return tx.Create(&databaseMigrationRecord{MigrationID: step.ID, Checksum: step.Checksum, ReleaseID: r.ReleaseID, RunUUID: &run.RunUUID, AppliedAt: time.Now().UTC()}).Error
			}
			var stepErr error
			if step.Transactional {
				stepErr = conn.Transaction(execute)
			} else {
				stepErr = execute(conn)
			}
			if stepErr != nil {
				out.Status = "failed"
				out.ErrorCode = "UPGRADE_STEP_FAILED"
				out.ErrorCode, out.DatabaseErrorCode = safeUpgradeError(stepErr)
				var pgError *pgconn.PgError
				if errors.As(stepErr, &pgError) && regexp.MustCompile(`^[a-zA-Z0-9_]{1,128}$`).MatchString(pgError.ConstraintName) {
					out.DatabaseConstraint = pgError.ConstraintName
				}
				out.Message = "Migration failed; inspect the step identifier and recovery policy"
				out.Failed = append(out.Failed, step.ID)
				out.Partial = len(out.Applied)+len(out.Skipped) > 0 || !step.Transactional
				out.Retryable = step.Transactional
				out.RequiresRestore = !step.Transactional
				cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				return conn.WithContext(cleanup).Model(&run).Updates(map[string]any{"status": "failed", "error_code": out.ErrorCode, "database_error_code": out.DatabaseErrorCode, "database_constraint": out.DatabaseConstraint, "partial": out.Partial, "retryable": out.Retryable, "requires_restore": out.RequiresRestore, "finished_at": time.Now().UTC()}).Error
			}
			out.Applied = append(out.Applied, step.ID)
			out.Pending = out.Pending[1:]
		}
		out.Pending = []UpgradeStep{}
		out.Status = "succeeded"
		out.CurrentRelease = r.ReleaseID
		out.CurrentMigrationVersion = r.MigrationVersion
		return conn.Model(&run).Updates(map[string]any{"status": "succeeded", "migration_id": "", "finished_at": time.Now().UTC()}).Error
	})
	if errors.Is(err, ErrMigrationLocked) {
		out.Block("UPGRADE_LOCK_BUSY", "Another migration owns the database lock")
		out.Retryable = true
		return out, nil
	}
	return out, err
}

// 仅允许平台常量错误码与 SQLSTATE；不保存 SQL 错误 Detail、DSN 或参数。
func safeUpgradeError(err error) (string, string) {
	code := "UPGRADE_STEP_FAILED"
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		if regexp.MustCompile(`^UPGRADE_[A-Z0-9_]{1,70}$`).MatchString(cause.Error()) {
			code = cause.Error()
			break
		}
	}
	var state interface{ SQLState() string }
	if errors.As(err, &state) && regexp.MustCompile(`^[A-Z0-9]{5}$`).MatchString(state.SQLState()) {
		return code, state.SQLState()
	}
	return code, ""
}
