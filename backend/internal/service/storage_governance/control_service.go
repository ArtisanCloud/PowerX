package storagegov

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ControlDomain string

const (
	ControlLifecycle         ControlDomain = "file-lifecycle"
	ControlBudget            ControlDomain = "host-budget"
	ControlBackupPolicies    ControlDomain = "database-backup-policies"
	ControlArchiveTargets    ControlDomain = "archive-targets"
	ControlArchivePolicies   ControlDomain = "archive-policies"
	ControlSnapshotPolicies  ControlDomain = "snapshot-policies"
	ControlDatabaseRetention ControlDomain = "database-retention"
	ControlTableRetention    ControlDomain = "system-table-retention"
	ControlTableMonitoring   ControlDomain = "table-monitoring"
	ControlExecution         ControlDomain = repo.ControlExecutionDomain
)
const controlQueueTenant = "platform:storage-control"
const controlTopic = "powerx.storage.configuration.apply.v1"

type ControlScope struct {
	HostID       string `json:"host_id"`
	FilesystemID string `json:"filesystem_id"`
	InstanceID   string `json:"instance_id"`
}

func (s ControlScope) ID() string {
	return controlDigest([]byte(s.HostID + "\x00" + s.FilesystemID + "\x00" + s.InstanceID))
}
func controlDigest(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }

type ControlPublication struct {
	Scope     ControlScope    `json:"scope"`
	ScopeID   string          `json:"scope_id"`
	Domain    ControlDomain   `json:"domain"`
	Revision  int64           `json:"revision"`
	Digest    string          `json:"digest"`
	Document  json.RawMessage `json:"document"`
	ActorID   uint64          `json:"actor_id"`
	UpdatedAt time.Time       `json:"updated_at"`
}
type ControlBinding struct {
	// Commit 仅为固定域耦合既有业务配置表；与版本、回执及 TaskBus 登记同事务，不执行外部 IO。
	Commit func(context.Context, *gorm.DB, json.RawMessage) error
	// Validate 必须绑定固定业务 DTO，并拒绝未知字段；不存在通用 HTTP JSON 写入口。
	Validate func(json.RawMessage) error
	// Apply 只能发布当前域的受控配置快照，不允许执行备份、归档或删除业务文件。
	Apply func(context.Context, ControlPublication) error
}
type ControlOptions struct {
	Scope ControlScope
	Queue *taskqueue.StreamDriver
	// Dispatch 的正式实现为同一 StreamDriver.Enqueue；测试可注入断网错误。
	Dispatch func(context.Context, event_bus.TaskMessage) error
	Changed  func()
}
type ControlInput struct {
	RequestID        string
	ExpectedRevision int64
	Domain           ControlDomain
	Document         json.RawMessage
}
type ControlState struct {
	Scope             ControlScope    `json:"scope"`
	Domain            ControlDomain   `json:"domain"`
	DesiredRevision   int64           `json:"desired_revision"`
	AppliedRevision   int64           `json:"applied_revision"`
	Digest            string          `json:"digest"`
	State             string          `json:"state"`
	Problem           string          `json:"problem,omitempty"`
	Document          json.RawMessage `json:"document"`
	OperationRevision int64           `json:"operation_revision,omitempty"`
	RequestID         string          `json:"request_id,omitempty"`
	Replayed          bool            `json:"replayed"`
	ActorID           uint64          `json:"actor_id"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// ControlService 负责平台配置事务、正式 TaskBus 发布和执行端版本确认。
// 本服务不创建连接、不自动迁移、不使用业务轮询队列。
type ControlService struct {
	repo     *repo.ControlRepository
	opts     ControlOptions
	bindings map[ControlDomain]ControlBinding
	started  bool
	mu       sync.RWMutex
	// 仅唤醒正式 TaskBus 恢复；消息及业务意图始终以数据库登记为准。
	recoverWake    chan struct{}
	done           chan struct{}
	executionFence func(context.Context, *gorm.DB, *m.ControlConfiguration) error
	executionGuard func(context.Context, func() error) error
}

func NewControlService(db *gorm.DB, opts ControlOptions) (*ControlService, error) {
	if db == nil || !idPattern.MatchString(opts.Scope.HostID) || !idPattern.MatchString(opts.Scope.FilesystemID) || !idPattern.MatchString(opts.Scope.InstanceID) {
		return nil, controlError(400, "STORAGE_CONTROL_SCOPE_INVALID")
	}
	if opts.Dispatch == nil && opts.Queue != nil {
		opts.Dispatch = opts.Queue.Enqueue
	}
	return &ControlService{repo: repo.NewControlRepository(db), opts: opts, bindings: map[ControlDomain]ControlBinding{}, recoverWake: make(chan struct{}, 1), done: make(chan struct{})}, nil
}
func controlError(status int, code string) error {
	return dto.NewErrorWithCode(status, code, code, nil)
}
func knownControlDomain(domain ControlDomain) bool {
	if strings.HasPrefix(string(domain), "file-lifecycle.") && len(domain) == len("file-lifecycle.")+32 {
		_, e := hex.DecodeString(strings.TrimPrefix(string(domain), "file-lifecycle."))
		return e == nil
	}
	for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
		if domain == systemRetentionDomain(id) {
			return true
		}
	}
	switch domain {
	case ControlExecution, ControlBackupPolicies, ControlLifecycle, ControlBudget, ControlArchiveTargets, ControlArchivePolicies, ControlSnapshotPolicies, ControlDatabaseRetention, ControlTableRetention, ControlTableMonitoring:
		return true
	}
	return false
}

// WithExecutionTx 在受控步骤前先核对全局执行开关，再锁定资源版本；不得包住长任务。
func (s *ControlService) WithExecutionTx(ctx context.Context, tx *gorm.DB, domain ControlDomain, fn func(*m.ControlConfiguration) error) error {
	if domain == ControlExecution {
		return controlError(400, "STORAGE_CONTROL_DOMAIN_INVALID")
	}
	run := func() error {
		return s.repo.WithExecutionTx(ctx, tx, s.opts.Scope.ID(), func(row *m.ControlConfiguration) error {
			if s.executionFence != nil {
				if e := s.executionFence(ctx, tx, row); e != nil {
					return e
				}
			}
			return s.repo.WithCurrentTx(ctx, tx, s.opts.Scope.ID(), string(domain), fn)
		})
	}
	if s.executionGuard != nil {
		return s.executionGuard(ctx, run)
	}
	return run()
}
func (s *ControlService) WithExecution(ctx context.Context, domain ControlDomain, fn func(*m.ControlConfiguration) error) error {
	return s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.WithExecutionTx(ctx, tx, domain, fn) })
}
func (s *ControlService) Bind(domain ControlDomain, binding ControlBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || !knownControlDomain(domain) || binding.Validate == nil || binding.Apply == nil {
		return controlError(400, "STORAGE_CONTROL_BINDING_INVALID")
	}
	if _, exists := s.bindings[domain]; exists {
		return controlError(409, "STORAGE_CONTROL_BINDING_EXISTS")
	}
	s.bindings[domain] = binding
	return nil
}
func (s *ControlService) binding(domain ControlDomain) (ControlBinding, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.bindings[domain]
	return b, ok
}
func (s *ControlService) Ready(ctx context.Context) bool {
	return s.CheckReady(ctx) == nil
}

// CheckReady 区分治理结构、数据库依赖及正式 TaskBus 装配，不隐式修复。
func (s *ControlService) CheckReady(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return controlError(503, "STORAGE_GOVERNANCE_UNAVAILABLE")
	}
	if err := s.repo.CheckReady(ctx); err != nil {
		return admissionUnavailable(err)
	}
	if s.opts.Queue == nil || s.opts.Dispatch == nil {
		return controlError(503, "STORAGE_CONTROL_WORKER_UNAVAILABLE")
	}
	return nil
}
func (s *ControlService) state(row *m.ControlConfiguration) ControlState {
	return ControlState{Scope: s.opts.Scope, Domain: ControlDomain(row.Domain), DesiredRevision: row.Revision, AppliedRevision: row.AppliedRevision, Digest: row.Digest, State: row.PublicationState, Problem: row.Problem, Document: json.RawMessage(row.Document), ActorID: row.ActorID, UpdatedAt: row.UpdatedAt}
}
func (s *ControlService) read(ctx context.Context, domain ControlDomain) (ControlState, error) {
	row, e := s.repo.Get(ctx, s.opts.Scope.ID(), string(domain))
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return ControlState{Scope: s.opts.Scope, Domain: domain, State: "unconfigured"}, nil
	}
	if e != nil {
		return ControlState{}, controlError(503, "STORAGE_CONTROL_UNAVAILABLE")
	}
	if row.HostID != s.opts.Scope.HostID || row.FilesystemID != s.opts.Scope.FilesystemID || row.InstanceID != s.opts.Scope.InstanceID {
		return ControlState{}, controlError(409, "STORAGE_CONTROL_SCOPE_MISMATCH")
	}
	body, e := verifiedControlDocument(row.Document, row.Digest)
	if e != nil {
		return ControlState{}, e
	}
	row.Document = datatypes.JSON(body)
	return s.state(row), nil
}
func (s *ControlService) Read(ctx context.Context, domain ControlDomain) (ControlState, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return ControlState{}, ErrRootRequired
	}
	if !knownControlDomain(domain) {
		return ControlState{}, controlError(400, "STORAGE_CONTROL_DOMAIN_INVALID")
	}
	if err := s.CheckReady(ctx); err != nil {
		return ControlState{}, err
	}
	return s.read(ctx, domain)
}

func canonicalControl(body []byte) ([]byte, error) {
	if len(body) == 0 || len(body) > 1<<20 {
		return nil, controlError(400, "STORAGE_CONTROL_DOCUMENT_INVALID")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if e := decoder.Decode(&value); e != nil {
		return nil, controlError(400, "STORAGE_CONTROL_DOCUMENT_INVALID")
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, controlError(400, "STORAGE_CONTROL_DOCUMENT_INVALID")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, controlError(400, "STORAGE_CONTROL_DOCUMENT_INVALID")
	}
	var inspect func(any, int) bool
	inspect = func(v any, depth int) bool {
		if depth > 32 {
			return false
		}
		switch t := v.(type) {
		case map[string]any:
			for k, x := range t {
				switch strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(k), "_", ""), "-", "") {
				case "accesskey", "secretkey", "sessiontoken", "password", "privatekey", "databasedsn", "dsn", "token":
					return false
				}
				if !inspect(x, depth+1) {
					return false
				}
			}
		case []any:
			for _, x := range t {
				if !inspect(x, depth+1) {
					return false
				}
			}
		}
		return true
	}
	if !inspect(value, 0) {
		return nil, controlError(400, "STORAGE_CONTROL_SECRET_FORBIDDEN")
	}
	// PostgreSQL JSONB 会规范化数字及空白；摘要不能依赖入库前的原始字节。
	var numbers func(any) (any, error)
	numbers = func(v any) (any, error) {
		switch x := v.(type) {
		case json.Number:
			// 有界精度足以覆盖所有配置单位，同时阻止超大指数耗尽内存。
			if len(x.String()) > 128 {
				return nil, controlError(400, "STORAGE_CONTROL_NUMBER_INVALID")
			}
			if index := strings.IndexAny(x.String(), "eE"); index >= 0 {
				exponent, e := strconv.Atoi(x.String()[index+1:])
				if e != nil || exponent < -1000 || exponent > 1000 {
					return nil, controlError(400, "STORAGE_CONTROL_NUMBER_INVALID")
				}
			}
			n, ok := new(big.Rat).SetString(x.String())
			if !ok || n.Num().BitLen() > 4096 || n.Denom().BitLen() > 4096 {
				return nil, controlError(400, "STORAGE_CONTROL_NUMBER_INVALID")
			}
			normalized := strings.TrimSuffix(strings.TrimRight(n.FloatString(1234), "0"), ".")
			if len(normalized) > 128 {
				return nil, controlError(400, "STORAGE_CONTROL_NUMBER_INVALID")
			}
			return json.Number(normalized), nil
		case map[string]any:
			for k, item := range x {
				out, e := numbers(item)
				if e != nil {
					return nil, e
				}
				x[k] = out
			}
		case []any:
			for k, item := range x {
				out, e := numbers(item)
				if e != nil {
					return nil, e
				}
				x[k] = out
			}
		}
		return v, nil
	}
	value, e := numbers(value)
	if e != nil {
		return nil, e
	}
	return json.Marshal(value)
}

func verifiedControlDocument(raw []byte, digest string) ([]byte, error) {
	body, e := canonicalControl(raw)
	if e != nil || controlDigest(body) != digest {
		return nil, controlError(409, "STORAGE_CONTROL_DIGEST_MISMATCH")
	}
	return body, nil
}
func (s *ControlService) subscriber() string { return "core.storage.control." + s.opts.Scope.ID() }
func (s *ControlService) message(version *m.ControlRevision) event_bus.TaskMessage {
	payload, _ := json.Marshal(struct {
		ScopeID  string        `json:"scope_id"`
		Domain   ControlDomain `json:"domain"`
		Revision int64         `json:"revision"`
		Digest   string        `json:"digest"`
	}{version.ScopeID, ControlDomain(version.Domain), version.Revision, version.Digest})
	// 配置请求的幂等已由数据库回执负责；TaskBus 按任务 UUID 去重。
	// 恢复副本有独立投递编号，不能复用原信封的不可变幂等键。
	return event_bus.TaskMessage{ID: version.TaskID, TenantKey: controlQueueTenant, SubscriberID: s.subscriber(), Topic: controlTopic, Payload: payload, TraceID: version.TraceID, Metadata: map[string]string{"kind": "storage_configuration", "max_attempts": "5"}}
}
func (s *ControlService) Save(ctx context.Context, in ControlInput) (ControlState, error) {
	if in.Domain == ControlExecution && s.executionGuard != nil {
		var out ControlState
		e := s.executionGuard(ctx, func() error { var e error; out, e = s.save(ctx, in); return e })
		return out, e
	}
	return s.save(ctx, in)
}
func (s *ControlService) save(ctx context.Context, in ControlInput) (ControlState, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return ControlState{}, ErrRootRequired
	}
	binding, ok := s.binding(in.Domain)
	if !ok {
		return ControlState{}, controlError(400, "STORAGE_CONTROL_DOMAIN_INVALID")
	}
	if _, e := uuid.Parse(in.RequestID); e != nil || len(in.RequestID) != 36 || in.ExpectedRevision < 0 {
		return ControlState{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	body, e := canonicalControl(in.Document)
	if e != nil {
		return ControlState{}, e
	}
	// 普通编辑延续已核对的恢复来源证明；客户端不能提供或替换它。
	if in.Domain != ControlExecution && in.ExpectedRevision > 0 {
		previous, err := s.repo.Revision(ctx, s.opts.Scope.ID(), string(in.Domain), in.ExpectedRevision)
		if err == nil && recoveryStamp(previous.Document) != nil {
			var document map[string]json.RawMessage
			if json.Unmarshal(body, &document) != nil {
				return ControlState{}, ErrStateUnavailable
			}
			document["recovery"], _ = json.Marshal(recoveryStamp(previous.Document))
			body, _ = json.Marshal(document)
			body, e = canonicalControl(body)
			if e != nil {
				return ControlState{}, e
			}
		}
	}
	if e = binding.Validate(body); e != nil {
		return ControlState{}, e
	}
	if !validRecoveryStamp(recoveryStamp(body)) {
		return ControlState{}, ErrStateUnavailable
	}
	if err := s.CheckReady(ctx); err != nil {
		return ControlState{}, err
	}
	actor := reqctx.GetClaims(ctx).UserID
	digest := controlDigest(body)
	requestDigest := controlDigest([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%s", s.opts.Scope.ID(), in.Domain, in.ExpectedRevision, actor, digest)))
	version := &m.ControlRevision{TenantID: 0, ScopeID: s.opts.Scope.ID(), Domain: string(in.Domain), Revision: in.ExpectedRevision + 1, RequestID: in.RequestID, RequestDigest: requestDigest, Document: datatypes.JSON(body), Digest: digest, ActorID: actor, TraceID: reqctx.GetTraceID(ctx), TaskID: uuid.NewString()}
	current := &m.ControlConfiguration{TenantID: 0, ScopeID: s.opts.Scope.ID(), Domain: string(in.Domain), HostID: s.opts.Scope.HostID, FilesystemID: s.opts.Scope.FilesystemID, InstanceID: s.opts.Scope.InstanceID, Revision: version.Revision, Document: datatypes.JSON(body), Digest: digest, PublicationState: "pending", ActorID: actor}
	receipt, replayed, e := s.repo.Save(ctx, current, version, in.ExpectedRevision, func(run context.Context, tx *gorm.DB) error {
		if binding.Commit != nil {
			if e := binding.Commit(run, tx, body); e != nil {
				return e
			}
		}
		return s.opts.Queue.Register(run, tx, s.message(version))
	})
	if errors.Is(e, repo.ErrRevisionConflict) {
		return ControlState{}, controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
	}
	if errors.Is(e, repo.ErrControlIdempotencyConflict) {
		return ControlState{}, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	if e != nil {
		if dto.CodeOf(e) != "" {
			return ControlState{}, e
		}
		return ControlState{}, controlError(503, "STORAGE_CONTROL_SAVE_UNAVAILABLE")
	}
	// 入队不等于生效；入队失败仍保留已提交的意图，正式驱动启动恢复时补投。
	if !replayed {
		dispatch, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		if s.opts.Dispatch(dispatch, s.message(receipt)) != nil {
			select {
			case s.recoverWake <- struct{}{}:
			default:
			}
		}
		cancel()
	}
	out, e := s.read(ctx, in.Domain)
	if e != nil {
		return ControlState{}, e
	}
	out.OperationRevision = receipt.Revision
	out.RequestID = in.RequestID
	out.Replayed = replayed
	if s.opts.Changed != nil {
		s.opts.Changed()
	}
	return out, nil
}

// ApplyDelivery 只由固定订阅者调用，在正式 TaskBus 完成事务内确认本地版本。
func (s *ControlService) ApplyDelivery(ctx context.Context, delivery event_bus.StreamTaskDelivery) error {
	msg := delivery.Message
	if msg.TenantKey != controlQueueTenant || msg.SubscriberID != s.subscriber() || msg.Topic != controlTopic {
		return controlError(400, "STORAGE_CONTROL_TASK_INVALID")
	}
	var input struct {
		ScopeID  string        `json:"scope_id"`
		Domain   ControlDomain `json:"domain"`
		Revision int64         `json:"revision"`
		Digest   string        `json:"digest"`
	}
	dec := json.NewDecoder(bytes.NewReader(msg.Payload))
	dec.DisallowUnknownFields()
	if dec.Decode(&input) != nil || input.ScopeID != s.opts.Scope.ID() || input.Revision < 1 {
		return controlError(400, "STORAGE_CONTROL_TASK_INVALID")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return controlError(400, "STORAGE_CONTROL_TASK_INVALID")
	}
	binding, ok := s.binding(input.Domain)
	if !ok {
		return controlError(400, "STORAGE_CONTROL_DOMAIN_INVALID")
	}
	version, e := s.repo.Revision(ctx, input.ScopeID, string(input.Domain), input.Revision)
	if e != nil || version.Digest != input.Digest {
		return controlError(409, "STORAGE_CONTROL_DIGEST_MISMATCH")
	}
	if _, e = verifiedControlDocument(version.Document, input.Digest); e != nil {
		return e
	}
	recordID, recordTenant := msg.Metadata["taskbus_record_id"], msg.Metadata["taskbus_record_tenant"]
	if recordID == "" {
		recordID = msg.ID
	}
	if recordTenant == "" {
		recordTenant = msg.TenantKey
	}
	if recordID != version.TaskID || recordTenant != controlQueueTenant {
		return controlError(409, "STORAGE_CONTROL_TASK_IDENTITY_MISMATCH")
	}
	e = s.opts.Queue.Complete(ctx, delivery, func(tx *gorm.DB) error {
		_, err := s.repo.Apply(ctx, tx, input.ScopeID, string(input.Domain), input.Revision, input.Digest, func(current m.ControlConfiguration) error {
			if current.ActorID != version.ActorID {
				return controlError(409, "STORAGE_CONTROL_DIGEST_MISMATCH")
			}
			if current.HostID != s.opts.Scope.HostID || current.FilesystemID != s.opts.Scope.FilesystemID || current.InstanceID != s.opts.Scope.InstanceID {
				return controlError(409, "STORAGE_CONTROL_SCOPE_MISMATCH")
			}
			body, e := verifiedControlDocument(current.Document, input.Digest)
			if e != nil {
				return e
			}
			return binding.Apply(ctx, ControlPublication{Scope: s.opts.Scope, ScopeID: input.ScopeID, Domain: input.Domain, Revision: input.Revision, Digest: input.Digest, Document: json.RawMessage(body), ActorID: current.ActorID, UpdatedAt: version.CreatedAt})
		})
		return err
	})
	if e != nil {
		_ = s.repo.Fail(context.WithoutCancel(ctx), input.ScopeID, string(input.Domain), input.Revision, "STORAGE_CONTROL_APPLY_FAILED")
	} else {
		// 完成持久化后才 ACK；ACK 失败不能把已生效版本倒写成失败。
		e = s.opts.Queue.Ack(ctx, delivery)
	}
	if s.opts.Changed != nil {
		s.opts.Changed()
	}
	return e
}
func (s *ControlService) Start(ctx context.Context, onError func(error)) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	go func() {
		defer close(s.done)
		if err := s.CheckReady(ctx); err != nil {
			if onError != nil {
				onError(err)
			}
			return
		}
		needsRecovery := true
		for ctx.Err() == nil {
			select {
			case <-s.recoverWake:
				needsRecovery = true
			default:
			}
			if needsRecovery {
				if e := s.opts.Queue.Recover(ctx, controlQueueTenant, s.subscriber()); e != nil {
					if onError != nil {
						onError(e)
					}
					if !controlBackoff(ctx) {
						return
					}
					continue
				}
				needsRecovery = false
			}
			deliveries, e := s.opts.Queue.Dequeue(ctx, controlQueueTenant, s.subscriber(), "storage-control-v1", s.opts.Scope.InstanceID, 1, 5*time.Second)
			if e != nil {
				if ctx.Err() != nil {
					return
				}
				if onError != nil {
					onError(e)
				}
				needsRecovery = true
				if !controlBackoff(ctx) {
					return
				}
				continue
			}
			for _, delivery := range deliveries {
				if e = s.ApplyDelivery(ctx, delivery); e != nil {
					_ = s.opts.Queue.Reject(ctx, delivery, "STORAGE_CONTROL_APPLY_FAILED", false)
					if onError != nil {
						onError(e)
					}
				}
			}
		}
	}()
}

func controlBackoff(ctx context.Context) bool {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Done 在已启动的配置消费者退出后关闭，供宿主关机及隔离验收确认资源释放。
func (s *ControlService) Done() <-chan struct{} { return s.done }

// ConsumerExited 用于报告曾启动但已经退出的消费者，不能以配置开关冒充正在发布。
func (s *ControlService) ConsumerExited() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}
