package runtime_host

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	cache "github.com/ArtisanCloud/PowerX/internal/infra/cache/runtime_host"
	capability "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_host"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/runtime_host"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	CacheRead           = "com.corex.runtime.cache.read"
	CacheManage         = "com.corex.runtime.cache.manage"
	TaskRead            = "com.corex.runtime.taskcenter.read"
	TaskManage          = "com.corex.runtime.taskcenter.manage"
	MaxValueBytes       = 1 << 20
	MaxJSONBytes        = 256 << 10
	MaxTTLMillis  int64 = 86400000
)

type Error struct {
	Kind  string
	Cause error
}

func (e *Error) Error() string               { return "runtime_host." + e.Kind }
func (e *Error) Unwrap() error               { return e.Cause }
func failure(kind string, cause error) error { return &Error{Kind: kind, Cause: cause} }
func Invalid() error                         { return failure("invalid_argument", nil) }

// Scope 只能由受信任 claims 与即时授权结果生成。
type scope struct{ tenant, subject uuid.UUID }
type Service struct {
	repo   *repo.Repository
	access *capability.DirectGrantService
	cache  *cache.Store
}

func NewService(db *gorm.DB, client redis.UniversalClient) *Service {
	return &Service{repo: repo.NewRepository(db), access: capability.NewDirectGrantService(db), cache: cache.NewStore(client)}
}
func (s *Service) authorize(ctx context.Context, capID string) (scope, error) {
	var out scope
	c := reqctx.GetClaims(ctx)
	if c == nil || c.Issuer != "powerx-sts" || len(c.Platforms) != 0 || c.UserID != 0 || c.MemberID != 0 || c.Scope != "access" {
		return out, failure("unauthorized", nil)
	}
	tenant, e := uuid.Parse(c.TenantUUID)
	if e != nil || tenant == uuid.Nil || tenant.String() != c.TenantUUID || reqctx.GetTenantUUID(ctx) != c.TenantUUID {
		return out, failure("unauthorized", nil)
	}
	if s == nil || s.access == nil {
		return out, failure("upstream_dependency", nil)
	}
	e = s.access.AuthorizeSTS(ctx, capID)
	if e != nil {
		var grant *capability.DirectGrantError
		if errors.As(e, &grant) {
			switch grant.Status {
			case 401:
				return out, failure("unauthorized", e)
			case 403:
				return out, failure("forbidden", e)
			}
		}
		return out, failure("upstream_dependency", e)
	}
	identity, _ := json.Marshal([]string{c.Issuer, c.PluginID, c.Subject})
	if len(identity) > 512 {
		return out, failure("unauthorized", nil)
	}
	subject, e := s.repo.Subject(ctx, tenant, string(identity))
	if e != nil || subject == uuid.Nil {
		return out, failure("upstream_dependency", e)
	}
	return scope{tenant, subject}, nil
}
func mapStorage(err error) error {
	if err == nil {
		return nil
	}
	var typed *Error
	if errors.As(err, &typed) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure("not_found", err)
	}
	if errors.Is(err, repo.ErrConflict) {
		return failure("conflict", err)
	}
	return failure("upstream_dependency", err)
}
func textValid(s string, n int) bool {
	return s != "" && len(s) <= n && utf8.ValidString(s) && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n")
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]*$`)

func UUID(s string) (uuid.UUID, error) {
	id, e := uuid.Parse(s)
	if e != nil || id == uuid.Nil || id.String() != s {
		return uuid.Nil, Invalid()
	}
	return id, nil
}

type CacheEntry struct {
	Found       bool       `json:"found"`
	ValueBase64 string     `json:"value_base64"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

func cacheKey(sc scope, namespace, key string) (string, error) {
	if !textValid(namespace, 128) || !textValid(key, 512) {
		return "", Invalid()
	}
	return cache.Key(sc.tenant.String(), sc.subject.String(), namespace, key), nil
}
func (s *Service) GetCache(ctx context.Context, namespace, key string) (*CacheEntry, error) {
	sc, e := s.authorize(ctx, CacheRead)
	if e != nil {
		return nil, e
	}
	k, e := cacheKey(sc, namespace, key)
	if e != nil {
		return nil, e
	}
	entry, e := s.cache.Get(ctx, k)
	if e != nil {
		return nil, mapStorage(e)
	}
	out := &CacheEntry{Found: entry.Found, ValueBase64: base64.StdEncoding.EncodeToString(entry.Value)}
	if entry.Found {
		out.ExpiresAt = &entry.ExpiresAt
	}
	return out, nil
}
func (s *Service) SetCache(ctx context.Context, namespace, key, value string, ttl int64) error {
	sc, e := s.authorize(ctx, CacheManage)
	if e != nil {
		return e
	}
	k, e := cacheKey(sc, namespace, key)
	if e != nil {
		return e
	}
	if ttl < 1 || ttl > MaxTTLMillis || len(value) > base64.StdEncoding.EncodedLen(MaxValueBytes) {
		return Invalid()
	}
	raw, e := base64.StdEncoding.Strict().DecodeString(value)
	if e != nil || len(raw) > MaxValueBytes || base64.StdEncoding.EncodeToString(raw) != value {
		return Invalid()
	}
	return s.cacheMutation(ctx, sc, "cache.set", k, func() error { return s.cache.Set(ctx, k, raw, time.Duration(ttl)*time.Millisecond) })
}
func (s *Service) DeleteCache(ctx context.Context, namespace, key string) error {
	sc, e := s.authorize(ctx, CacheManage)
	if e != nil {
		return e
	}
	k, e := cacheKey(sc, namespace, key)
	if e != nil {
		return e
	}
	return s.cacheMutation(ctx, sc, "cache.delete", k, func() error { return s.cache.Delete(ctx, k) })
}
func (s *Service) cacheMutation(ctx context.Context, sc scope, action, key string, fn func() error) error {
	// Redis 与数据库不是分布式事务：先持久化尝试，完成事件失败明确返回依赖错误。
	if e := s.repo.Audit(ctx, nil, sc.tenant, sc.subject, nil, action, "attempt", key); e != nil {
		return mapStorage(e)
	}
	e := fn()
	outcome := "succeeded"
	if e != nil {
		outcome = "failed"
	}
	auditErr := s.repo.Audit(ctx, nil, sc.tenant, sc.subject, nil, action, outcome, key)
	logger.InfoF(logger.WithLogFields(ctx, map[string]interface{}{"module": "runtime_host", "tenant_uuid": sc.tenant.String(), "caller_subject_uuid": sc.subject.String(), "operation": action, "outcome": outcome}), "runtime_host.operation")
	if e != nil {
		return mapStorage(e)
	}
	return mapStorage(auditErr)
}

type CreateTaskInput struct {
	Type           string          `json:"type"`
	IdempotencyKey string          `json:"idempotency_key"`
	Payload        json.RawMessage `json:"payload"`
}
type UpdateTaskInput struct {
	ExpectedRevision int64           `json:"expected_revision"`
	State            string          `json:"state"`
	Progress         int             `json:"progress"`
	MessageKey       string          `json:"message_key"`
	Result           json.RawMessage `json:"result"`
}
type Task struct {
	TaskUUID    uuid.UUID       `json:"task_uuid"`
	TenantUUID  uuid.UUID       `json:"tenant_uuid"`
	Type        string          `json:"type"`
	State       string          `json:"state"`
	Progress    int             `json:"progress"`
	Revision    int64           `json:"revision"`
	MessageKey  string          `json:"message_key"`
	Result      json.RawMessage `json:"result"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	CompletedAt *time.Time      `json:"completed_at"`
}

func view(row *m.Task) *Task {
	return &Task{row.UUID, row.TenantUUID, row.Type, row.State, row.Progress, row.Revision, row.MessageKey, json.RawMessage(row.Result), row.CreatedAt, row.UpdatedAt, row.CompletedAt}
}

// CanonicalJSON 拒绝重复字段，保留数字精度；对象键顺序和空白不影响幂等。
func CanonicalJSON(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || len(raw) > MaxJSONBytes || !utf8.Valid(raw) {
		return nil, Invalid()
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if e := checkJSON(d, 0); e != nil {
		return nil, e
	}
	if _, e := d.Token(); e != io.EOF {
		return nil, Invalid()
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value any
	if e := d.Decode(&value); e != nil {
		return nil, Invalid()
	}
	return json.Marshal(value)
}
func checkJSON(d *json.Decoder, depth int) error {
	if depth > 64 {
		return Invalid()
	}
	token, e := d.Token()
	if e != nil {
		return Invalid()
	}
	if value, ok := token.(string); ok && strings.ContainsRune(value, '\x00') {
		return Invalid()
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return Invalid()
			}
			key, ok := k.(string)
			if !ok || seen[key] || strings.ContainsRune(key, '\x00') {
				return Invalid()
			}
			seen[key] = true
			if e = checkJSON(d, depth+1); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if e = checkJSON(d, depth+1); e != nil {
				return e
			}
		}
	default:
		return Invalid()
	}
	_, e = d.Token()
	if e != nil {
		return Invalid()
	}
	return nil
}
func (s *Service) CreateTask(ctx context.Context, in CreateTaskInput) (*Task, error) {
	sc, e := s.authorize(ctx, TaskManage)
	if e != nil {
		return nil, e
	}
	if !textValid(in.Type, 128) || !namePattern.MatchString(in.Type) || !textValid(in.IdempotencyKey, 128) {
		return nil, Invalid()
	}
	payload, e := CanonicalJSON(in.Payload)
	if e != nil {
		return nil, e
	}
	fingerprint, _ := json.Marshal([]string{in.Type, string(payload)})
	hash := sha256.Sum256(fingerprint)
	now := time.Now().UTC()
	row := &m.Task{TenantUUID: sc.tenant, CallerSubjectUUID: sc.subject, Type: in.Type, IdempotencyKey: in.IdempotencyKey, Payload: datatypes.JSON(payload), RequestDigest: hex.EncodeToString(hash[:]), State: "queued", Revision: 1}
	row.CreatedAt = now
	row.UpdatedAt = now
	var saved *m.Task
	e = s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		var created bool
		var err error
		saved, created, err = s.repo.CreateTask(ctx, tx, row)
		if err != nil {
			return err
		}
		if saved.RequestDigest != row.RequestDigest {
			return repo.ErrConflict
		}
		if created {
			return s.repo.Audit(ctx, tx, sc.tenant, sc.subject, &saved.UUID, "task.create", "succeeded", "")
		}
		return nil
	})
	if e != nil {
		return nil, mapStorage(e)
	}
	return view(saved), nil
}
func (s *Service) GetTask(ctx context.Context, id string) (*Task, error) {
	sc, e := s.authorize(ctx, TaskRead)
	if e != nil {
		return nil, e
	}
	task, e := UUID(id)
	if e != nil {
		return nil, e
	}
	row, e := s.repo.GetTask(ctx, nil, sc.tenant, sc.subject, task)
	if e != nil {
		return nil, mapStorage(e)
	}
	return view(row), nil
}
func terminal(state string) bool {
	return state == "succeeded" || state == "failed" || state == "cancelled"
}
func validateTransition(row *m.Task, in UpdateTaskInput) error {
	if row.Revision != in.ExpectedRevision || row.Revision == math.MaxInt64 || terminal(row.State) || in.Progress < row.Progress {
		return repo.ErrConflict
	}
	if row.State == "queued" && in.State == "succeeded" || row.State == "running" && in.State == "queued" {
		return repo.ErrConflict
	}
	if row.State != "queued" && row.State != "running" {
		return failure("upstream_dependency", nil)
	}
	return nil
}
func (s *Service) UpdateTask(ctx context.Context, id string, in UpdateTaskInput) (*Task, error) {
	sc, e := s.authorize(ctx, TaskManage)
	if e != nil {
		return nil, e
	}
	task, e := UUID(id)
	if e != nil {
		return nil, e
	}
	if in.ExpectedRevision < 1 || in.Progress < 0 || in.Progress > 100 || (in.State != "queued" && in.State != "running" && !terminal(in.State)) || (in.State == "succeeded" && in.Progress != 100) || len(in.MessageKey) > 256 || (in.MessageKey != "" && !namePattern.MatchString(in.MessageKey)) {
		return nil, Invalid()
	}
	var result []byte
	if len(in.Result) > 0 {
		result, e = CanonicalJSON(in.Result)
		if e != nil {
			return nil, e
		}
	}
	var saved *m.Task
	e = s.repo.WithTx(ctx, func(tx *gorm.DB) error {
		row, err := s.repo.GetTask(ctx, tx, sc.tenant, sc.subject, task)
		if err != nil {
			return err
		}
		if err = validateTransition(row, in); err != nil {
			return err
		}
		row.State = in.State
		row.Progress = in.Progress
		row.Revision++
		row.MessageKey = in.MessageKey
		row.Result = datatypes.JSON(result)
		row.UpdatedAt = time.Now().UTC()
		if terminal(row.State) {
			row.CompletedAt = &row.UpdatedAt
		}
		if err = s.repo.UpdateTask(ctx, tx, row, in.ExpectedRevision); err != nil {
			return err
		}
		if err = s.repo.Audit(ctx, tx, sc.tenant, sc.subject, &row.UUID, "task.update", "succeeded", ""); err != nil {
			return err
		}
		saved = row
		return nil
	})
	if e != nil {
		return nil, mapStorage(e)
	}
	return view(saved), nil
}
