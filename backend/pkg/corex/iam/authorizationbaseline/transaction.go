package authorizationbaseline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/rootrole"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrWriteTransaction = errors.New("IAM_BASELINE_WRITE_TRANSACTION_REQUIRED")
var ErrUnsupportedDatabase = errors.New("IAM_BASELINE_UNSUPPORTED_DATABASE")

// 首期生产边界为 PostgreSQL；SQLite 仅用于隔离测试。
func SupportedDatabase(db *gorm.DB) bool {
	return db != nil && (db.Dialector.Name() == "postgres" || db.Dialector.Name() == "sqlite")
}

// transactionPool 将账本维护放在最外层 Commit 之前。所有正式 GORM/SQL 写入口共用该边界，
// 无需在每个 handler 里维护另一份授权。源事务或维护失败均回滚；普通查询没有副作用。
type transactionPool struct {
	gorm.ConnPool
	db *gorm.DB
}
type trackedTransaction struct {
	gorm.ConnPool
	pool      *transactionPool
	commit    gorm.TxCommitter
	ctx       context.Context
	isolation sql.IsolationLevel
	dirty     bool
	internal  bool
	ready     bool
}

// Install 在连接工厂安装一次；源事务提交时自动登记缺失追踪并维护账本，查询不写库。
func Install(db *gorm.DB) error {
	if db == nil {
		return errors.New("nil database")
	}
	if _, ok := db.Statement.ConnPool.(*transactionPool); ok {
		return nil
	}
	if _, ok := db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return ErrWriteTransaction
	}
	pool := &transactionPool{ConnPool: db.Statement.ConnPool, db: db}
	db.ConnPool = pool
	db.Statement.ConnPool = pool
	// Connection/SkipDefaultTransaction 等绕过事务的写法必须明确失败，不能漏记事实。
	guard := func(tx *gorm.DB) {
		if !watchedTable(tx.Statement.Table) && !sourceMutation(tx.Statement.SQL.String()) {
			return
		}
		if managed, ok := tx.Statement.ConnPool.(*trackedTransaction); !ok {
			tx.AddError(ErrWriteTransaction)
		} else if err := managed.beginWrite(tx.Statement.Context); err != nil {
			tx.AddError(err)
		}
	}
	if err := db.Callback().Create().Before("gorm:create").Register("iam:authorization_transaction", guard); err != nil {
		return err
	}
	if err := db.Callback().Update().Before("gorm:update").Register("iam:authorization_transaction", guard); err != nil {
		return err
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("iam:authorization_transaction", guard); err != nil {
		return err
	}
	if err := db.Callback().Query().Before("gorm:query").Register("iam:authorization_source_lock", func(tx *gorm.DB) {
		locking, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if ok && strings.EqualFold(locking.Strength, "UPDATE") && watchedTable(tx.Statement.Table) {
			guard(tx)
		}
	}); err != nil {
		return err
	}
	// Raw Exec 在连接池中原子包装；裸连接 Raw 不允许绕过。
	return db.Callback().Raw().Before("gorm:raw").Register("iam:authorization_transaction", func(tx *gorm.DB) {
		if sourceMutation(tx.Statement.SQL.String()) {
			switch tx.Statement.ConnPool.(type) {
			case *transactionPool, *trackedTransaction:
			default:
				tx.AddError(ErrWriteTransaction)
			}
		}
	})
}

func (p *transactionPool) GetDBConn() (*sql.DB, error) {
	if db, ok := p.ConnPool.(*sql.DB); ok {
		return db, nil
	}
	if d, ok := p.ConnPool.(gorm.GetDBConnector); ok {
		return d.GetDBConn()
	}
	if parent, ok := p.db.ConnPool.(*transactionPool); ok && parent != p {
		return parent.GetDBConn()
	}
	return nil, gorm.ErrInvalidDB
}

// BindConnection 保留显式借用连接上的事务协调，不修改根连接池。
func BindConnection(db *gorm.DB) {
	if _, managed := db.ConnPool.(*transactionPool); !managed {
		return
	}
	if _, already := db.Statement.ConnPool.(*transactionPool); already {
		return
	}
	db.Statement.ConnPool = &transactionPool{ConnPool: db.Statement.ConnPool, db: db}
}
func (p *transactionPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	var conn gorm.ConnPool
	var err error
	switch b := p.ConnPool.(type) {
	case gorm.TxBeginner:
		conn, err = b.BeginTx(ctx, opts)
	case gorm.ConnPoolBeginner:
		conn, err = b.BeginTx(ctx, opts)
	default:
		return nil, gorm.ErrInvalidTransaction
	}
	if err != nil {
		return nil, err
	}
	committer, ok := conn.(gorm.TxCommitter)
	if !ok {
		return nil, gorm.ErrInvalidTransaction
	}
	isolation := sql.LevelDefault
	if opts != nil {
		isolation = opts.Isolation
	}
	return &trackedTransaction{ConnPool: conn, pool: p, commit: committer, ctx: ctx, isolation: isolation}, nil
}
func (p *transactionPool) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	if !sourceMutation(q) {
		return p.ConnPool.ExecContext(ctx, q, args...)
	}
	conn, err := p.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	tx := conn.(*trackedTransaction)
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
func (p *transactionPool) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	if sourceMutation(q) {
		return nil, ErrWriteTransaction
	}
	return p.ConnPool.QueryContext(ctx, q, args...)
}
func (p *transactionPool) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	if sourceMutation(q) {
		failed, cancel := context.WithCancel(ctx)
		cancel()
		return p.ConnPool.QueryRowContext(failed, q, args...)
	}
	return p.ConnPool.QueryRowContext(ctx, q, args...)
}
func (p *transactionPool) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	if sourceMutation(q) {
		return nil, ErrWriteTransaction
	}
	return p.ConnPool.PrepareContext(ctx, q)
}

func (t *trackedTransaction) database() *gorm.DB {
	db := t.pool.db.Session(&gorm.Session{NewDB: true, SkipDefaultTransaction: true, Context: t.ctx})
	db.Statement.ConnPool = t
	return db
}
func (t *trackedTransaction) beginWrite(ctx context.Context) error {
	if t.dirty || t.internal {
		return nil
	}
	if t.isolation != sql.LevelDefault && t.isolation != sql.LevelReadCommitted {
		return errors.New("IAM_BASELINE_WRITE_REQUIRES_READ_COMMITTED")
	}
	// 首期使用平台级授权源门闩，覆盖跨租户权限目录和新增关系。获取后才枚举跟踪流。
	if t.pool.db.Dialector.Name() == "postgres" {
		if _, err := t.ConnPool.ExecContext(ctx, "SELECT pg_advisory_xact_lock(704983720161015)"); err != nil {
			return err
		}
	}
	db := t.database()
	t.internal = true
	defer func() { t.internal = false }()
	exists := db.Migrator().HasTable(&im.AuthorizationTracking{})
	if exists {
		if !db.Migrator().HasTable(&im.AuthorizationStream{}) || !db.Migrator().HasTable(&im.AuthorizationSegment{}) {
			return errors.New("IAM_BASELINE_SCHEMA_INCOMPLETE")
		}
		if !SupportedDatabase(db) {
			var ready int64
			if err := db.Model(&im.AuthorizationTracking{}).Where("status=?", "ready").Count(&ready).Error; err != nil {
				return err
			}
			if ready > 0 {
				return ErrUnsupportedDatabase
			}
		}
		if t.pool.db.Dialector.Name() == "sqlite" {
			if _, err := t.ConnPool.ExecContext(ctx, "UPDATE "+(&im.AuthorizationTracking{}).TableName()+" SET status=status WHERE 1=0"); err != nil {
				return err
			}
		}
	}
	t.ready = exists
	t.dirty = true
	return nil
}
func (t *trackedTransaction) ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error) {
	if sourceMutation(q) {
		if err := t.beginWrite(ctx); err != nil {
			return nil, err
		}
	}
	return t.ConnPool.ExecContext(ctx, q, args...)
}
func (t *trackedTransaction) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	if sourceMutation(q) {
		if err := t.beginWrite(ctx); err != nil {
			return nil, err
		}
	}
	return t.ConnPool.QueryContext(ctx, q, args...)
}
func (t *trackedTransaction) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	if sourceMutation(q) {
		if err := t.beginWrite(ctx); err != nil {
			failed, cancel := context.WithCancel(ctx)
			cancel()
			return t.ConnPool.QueryRowContext(failed, q, args...)
		}
	}
	return t.ConnPool.QueryRowContext(ctx, q, args...)
}
func (t *trackedTransaction) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	if sourceMutation(q) {
		if err := t.beginWrite(ctx); err != nil {
			return nil, err
		}
	}
	return t.ConnPool.PrepareContext(ctx, q)
}
func (t *trackedTransaction) StmtContext(ctx context.Context, stmt *sql.Stmt) *sql.Stmt {
	if tx, ok := t.ConnPool.(interface {
		StmtContext(context.Context, *sql.Stmt) *sql.Stmt
	}); ok {
		return tx.StmtContext(ctx, stmt)
	}
	return stmt
}
func (t *trackedTransaction) GetDBConn() (*sql.DB, error) { return t.pool.GetDBConn() }
func (t *trackedTransaction) Commit() error {
	if t.dirty && t.ready {
		t.internal = true
		if _, err := rootrole.Ensure(t.database()); err != nil {
			_ = t.commit.Rollback()
			return fmt.Errorf("IAM_ROOT_ROLE_SYNC_FAILED: %w", err)
		}
		at, err := transactionTime(t.database())
		if err != nil {
			_ = t.commit.Rollback()
			return err
		}
		if SupportedDatabase(t.database()) {
			if _, err = initializeMissing(t.database(), at); err != nil {
				_ = t.commit.Rollback()
				return fmt.Errorf("IAM_BASELINE_INITIALIZATION_FAILED: %w", err)
			}
		}
		if err := Reconcile(t.database(), at); err != nil {
			_ = t.commit.Rollback()
			return fmt.Errorf("IAM_BASELINE_RECONCILE_FAILED: %w", err)
		}
	}
	return t.commit.Commit()
}
func (t *trackedTransaction) Rollback() error { return t.commit.Rollback() }

var mutationTable = regexp.MustCompile("(?i)\\b(?:INSERT\\s+INTO|UPDATE|DELETE\\s+FROM|TRUNCATE(?:\\s+TABLE)?)\\s+((?:[\"`a-zA-Z0-9_]+\\.)?[\"`a-zA-Z0-9_]+)")
var sqlComment = regexp.MustCompile(`(?s)/\*.*?\*/|--[^\n]*`)

func sourceMutation(query string) bool {
	query = sqlComment.ReplaceAllString(query, " ")
	for _, m := range mutationTable.FindAllStringSubmatch(query, -1) {
		if watchedTable(m[1]) {
			return true
		}
	}
	return false
}
func watchedTable(table string) bool {
	table = strings.ReplaceAll(strings.ReplaceAll(table, "\"", ""), "`", "")
	if i := strings.LastIndex(table, "."); i >= 0 {
		table = table[i+1:]
	}
	models := []interface{ TableName() string }{&im.User{}, &im.Member{}, &im.Role{}, &im.Permission{}, &im.RolePermission{}, &im.RoleBinding{}, &im.MemberAssignment{}, &im.Department{}, &im.Team{}, &im.Position{}, &im.Group{}, &tm.Tenant{}, &sm.PluginInstanceConfig{}}
	for _, m := range models {
		name := m.TableName()
		if i := strings.LastIndex(name, "."); i >= 0 {
			name = name[i+1:]
		}
		if table == name {
			return true
		}
	}
	return false
}

// Lock 在业务读取/枚举之前取得共同源锁，供初始化及集合覆盖写入使用。
func Lock(db *gorm.DB) error {
	tx, ok := db.Statement.ConnPool.(*trackedTransaction)
	if !ok {
		return ErrWriteTransaction
	}
	return tx.beginWrite(db.Statement.Context)
}

func Managed(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	switch db.Statement.ConnPool.(type) {
	case *transactionPool, *trackedTransaction:
		return true
	}
	return false
}
func InTransaction(db *gorm.DB) bool {
	if db == nil {
		return false
	}
	_, ok := db.Statement.ConnPool.(*trackedTransaction)
	return ok
}
