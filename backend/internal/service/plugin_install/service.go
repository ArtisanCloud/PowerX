// Package plugininstall owns durable admin installation intents. All execution
// and delivery leases use the existing formal Stream TaskBus.
package plugininstall

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_install"
	r "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_install"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const queueTenant = "core-plugin-install"
const MaxSourceBytes int64 = 1 << 30

var targetPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,255}$`)
var versionPart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)

type Installer interface {
	InstallFromFile(context.Context, string, pm.InstallOptions) (pm.Plugin, error)
	InstallFromURL(context.Context, string, string, string, pm.InstallOptions) (pm.Plugin, error)
	Get(context.Context, string) (pm.Plugin, error)
	Enable(context.Context, string) error
}

type Options struct {
	Root, InstanceID string
	Secret           []byte
	Queue            *taskqueue.StreamDriver
	Manager          Installer
	Authorize        func(context.Context, string, uint64) error
	Finalize         func(context.Context, string, pm.Plugin) error
	Notify           func(context.Context, string)
	Timeout          time.Duration
	Disabled         bool
	RequireWorker    bool
}

type Input struct {
	RequestID             string             `json:"request_id"`
	SourceKind            string             `json:"source_kind"`
	PluginID              string             `json:"plugin_id"`
	Version               string             `json:"version"`
	URL                   string             `json:"url,omitempty"`
	SHA256                string             `json:"sha256,omitempty"`
	Enable                bool               `json:"enable"`
	Force                 bool               `json:"force"`
	ExpectedRevision      string             `json:"expected_revision,omitempty"`
	ConfirmLegacyProvider bool               `json:"confirm_legacy_provider"`
	Metadata              pm.InstallMetadata `json:"metadata"`
}

type intent struct {
	Input
	RequireAbsent bool `json:"_require_absent"`
}

type View struct {
	TaskID            string          `json:"task_id"`
	RequestID         string          `json:"request_id"`
	PluginID          string          `json:"plugin_id"`
	Version           string          `json:"version"`
	InstalledRevision string          `json:"installed_revision,omitempty"`
	Status            string          `json:"status"`
	Phase             string          `json:"phase"`
	Revision          int64           `json:"revision"`
	SourceKind        string          `json:"source_kind"`
	ErrorCode         string          `json:"error_code,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	CompletedAt       *time.Time      `json:"completed_at,omitempty"`
	FailureDetails    json.RawMessage `json:"failure_details,omitempty"`
}

func view(row *m.Installation) View {
	return View{row.UUID.String(), row.RequestID, row.PluginID, row.Version, row.InstalledRevision, row.Status, row.Phase, row.Revision, row.SourceKind, row.ErrorCode, row.CreatedAt, row.UpdatedAt, row.CompletedAt, json.RawMessage(row.FailureDetails)}
}
func problem(status int, code string) error { return dto.NewErrorWithCode(status, code, code, nil) }
func canonicalID(s string) bool {
	id, e := uuid.Parse(s)
	return e == nil && id != uuid.Nil && id.String() == s
}
func terminal(status string) bool {
	return status == "succeeded" || status == "failed" || status == "needs_attention" || status == "cancelled"
}

type Service struct {
	db         *gorm.DB
	repo       *r.InstallationRepository
	opts       Options
	scope      string
	subscriber string
	aead       cipher.AEAD
	wake       chan struct{}
	running    atomic.Bool
}

func NewService(db *gorm.DB, opts Options) (*Service, error) {
	if db == nil || opts.Root == "" || opts.InstanceID == "" || len(opts.Secret) < 32 || opts.Queue == nil || opts.Manager == nil || opts.Authorize == nil || opts.Finalize == nil {
		return nil, problem(503, "PLUGIN_INSTALL_TASKS_UNAVAILABLE")
	}
	root, e := filepath.Abs(opts.Root)
	if e != nil {
		return nil, e
	}
	opts.Root = root
	key := sha256.Sum256(opts.Secret)
	block, e := aes.NewCipher(key[:])
	if e != nil {
		return nil, e
	}
	aead, e := cipher.NewGCM(block)
	if e != nil {
		return nil, e
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 15 * time.Minute
	}
	scope := sha256.Sum256([]byte(opts.InstanceID + "\x00" + root))
	sc := hex.EncodeToString(scope[:])
	return &Service{db: db, repo: r.NewInstallationRepository(db), opts: opts, scope: sc, subscriber: "plugin-install-v1:" + sc[:32], aead: aead, wake: make(chan struct{}, 1)}, nil
}
func (s *Service) Ready() bool { return s != nil && s.repo.Ready() }
func (s *Service) actor(ctx context.Context) (string, uint64, error) {
	if !s.Ready() {
		return "", 0, problem(503, "PLUGIN_INSTALL_SCHEMA_NOT_READY")
	}
	c := reqctx.GetClaims(ctx)
	tenant, e := reqctx.CanonicalTenantUUID(reqctx.GetTenantUUID(ctx))
	if e != nil || c == nil || c.UserID == 0 || !canonicalID(c.UserUUID) || !reqctx.IsRoot(ctx) || (c.Issuer == "powerx-sts" || c.PluginID != "") {
		return "", 0, problem(403, "PLUGIN_INSTALL_ROOT_REQUIRED")
	}
	if e = s.opts.Authorize(ctx, tenant, c.UserID); e != nil {
		return "", 0, e
	}
	return tenant, c.UserID, nil
}
func (s *Service) EventChannel(tenant, id string) string {
	return "powerx:plugin-install:" + s.scope + ":" + tenant + ":" + id
}
func (s *Service) changed(ctx context.Context, row *m.Installation) {
	if s.opts.Notify != nil {
		s.opts.Notify(ctx, s.EventChannel(row.TenantUUID, row.UUID.String()))
	}
}
func (s *Service) seal(row *m.Installation, raw []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, e := io.ReadFull(rand.Reader, nonce); e != nil {
		return nil, e
	}
	return s.aead.Seal(nonce, nonce, raw, []byte(row.TenantUUID+":"+row.UUID.String())), nil
}
func (s *Service) input(row *m.Installation) (intent, error) {
	var out intent
	n := s.aead.NonceSize()
	if len(row.PrivateRequest) < n {
		return out, problem(409, "PLUGIN_INSTALL_INTENT_INVALID")
	}
	raw, e := s.aead.Open(nil, row.PrivateRequest[:n], row.PrivateRequest[n:], []byte(row.TenantUUID+":"+row.UUID.String()))
	if e != nil {
		return out, problem(409, "PLUGIN_INSTALL_INTENT_INVALID")
	}
	e = json.Unmarshal(raw, &out)
	return out, e
}
func (s *Service) Create(ctx context.Context, in Input) (View, error) {
	if s == nil || s.opts.Disabled || (s.opts.RequireWorker && !s.running.Load()) {
		return View{}, problem(503, "PLUGIN_INSTALL_WORKER_DISABLED")
	}
	tenant, actor, e := s.actor(ctx)
	if e != nil {
		return View{}, e
	}
	if !canonicalID(in.RequestID) || (in.SourceKind != "local" && in.SourceKind != "url") || len(in.Version) > 128 || (in.PluginID != "" && !targetPart.MatchString(in.PluginID)) || (in.Version != "" && !versionPart.MatchString(in.Version)) {
		return View{}, problem(400, "PLUGIN_INSTALL_TASK_INVALID")
	}
	if in.SourceKind == "local" && (in.PluginID == "" || in.Version == "" || in.URL != "") {
		return View{}, problem(400, "PLUGIN_INSTALL_TASK_INVALID")
	}
	if in.SourceKind == "url" {
		u, e := url.Parse(in.URL)
		if e != nil || u.User != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || len(in.URL) > 8192 {
			return View{}, problem(400, "PLUGIN_INSTALL_URL_INVALID")
		}
	}
	if in.SHA256 != "" {
		b, e := hex.DecodeString(in.SHA256)
		if e != nil || len(b) != 32 {
			return View{}, problem(400, "PLUGIN_INSTALL_CHECKSUM_INVALID")
		}
	}
	raw, e := json.Marshal(in)
	if e != nil || len(raw) > 32768 {
		return View{}, problem(400, "PLUGIN_INSTALL_TASK_INVALID")
	}
	sum := sha256.Sum256(raw)
	previous, lookupErr := s.repo.ByRequest(ctx, tenant, actor, in.RequestID)
	if lookupErr == nil {
		if previous.RequestDigest != hex.EncodeToString(sum[:]) || previous.ActorUUID != reqctx.GetClaims(ctx).UserUUID || previous.InstanceID != s.scope {
			return View{}, problem(409, "PLUGIN_INSTALL_REQUEST_CONFLICT")
		}
		if previous.Status == "queued" {
			s.dispatch(previous)
		}
		return view(previous), nil
	}
	if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return View{}, lookupErr
	}
	// Pin both presence and absence. A delayed local force upload must never
	// adopt a revision created by another task while it was waiting in the queue.
	requireAbsent := false
	if in.PluginID != "" {
		current, readErr := s.opts.Manager.Get(ctx, in.PluginID)
		if readErr == nil {
			if in.ExpectedRevision == "" {
				in.ExpectedRevision = current.Revision
			}
		} else if pm.IsCode(readErr, pm.CodeNotFound) {
			requireAbsent = true
		} else {
			return View{}, problem(503, "PLUGIN_INSTALL_REGISTRY_UNAVAILABLE")
		}
	}
	private, e := json.Marshal(intent{Input: in, RequireAbsent: requireAbsent})
	if e != nil {
		return View{}, e
	}
	row := &m.Installation{TenantUUID: tenant, ActorID: actor, ActorUUID: reqctx.GetClaims(ctx).UserUUID, RequestID: in.RequestID, InstanceID: s.scope, RequestDigest: hex.EncodeToString(sum[:]), SourceKind: in.SourceKind, PluginID: in.PluginID, Version: in.Version, Status: "awaiting_upload", Phase: "upload", Revision: 1}
	row.UUID = uuid.New()
	row.PrivateRequest, e = s.seal(row, private)
	if e != nil {
		return View{}, e
	}
	if in.SourceKind == "url" {
		row.Status = "queued"
		row.Phase = "queued"
	}
	inserted := false
	e = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		inserted, err = s.repo.Insert(ctx, tx, row)
		if err != nil {
			return err
		}
		if inserted && in.SourceKind == "url" {
			return s.opts.Queue.Register(ctx, tx, s.message(row))
		}
		return nil
	})
	if e != nil {
		return View{}, e
	}
	if !inserted {
		row, e = s.repo.ByRequest(ctx, tenant, actor, in.RequestID)
		if e != nil {
			return View{}, e
		}
		if row.RequestDigest != hex.EncodeToString(sum[:]) || row.ActorUUID != reqctx.GetClaims(ctx).UserUUID || row.InstanceID != s.scope {
			return View{}, problem(409, "PLUGIN_INSTALL_REQUEST_CONFLICT")
		}
	}
	if row.Status == "queued" {
		s.dispatch(row)
	}
	return view(row), nil
}
func (s *Service) owned(ctx context.Context, id string) (*m.Installation, error) {
	tenant, actor, e := s.actor(ctx)
	if e != nil {
		return nil, e
	}
	if !canonicalID(id) {
		return nil, problem(400, "PLUGIN_INSTALL_TASK_INVALID")
	}
	row, e := s.repo.Get(ctx, tenant, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, problem(404, "PLUGIN_INSTALL_TASK_NOT_FOUND")
	}
	if e != nil {
		return nil, e
	}
	if row.ActorID != actor || row.ActorUUID != reqctx.GetClaims(ctx).UserUUID || row.InstanceID != s.scope {
		return nil, problem(404, "PLUGIN_INSTALL_TASK_NOT_FOUND")
	}
	return row, nil
}
func (s *Service) Get(ctx context.Context, id string) (View, error) {
	row, e := s.owned(ctx, id)
	if e != nil {
		return View{}, e
	}
	return view(row), nil
}
func (s *Service) List(ctx context.Context) ([]View, error) {
	tenant, actor, e := s.actor(ctx)
	if e != nil {
		return nil, e
	}
	rows, e := s.repo.List(ctx, tenant, actor, reqctx.GetClaims(ctx).UserUUID, s.scope)
	if e != nil {
		return nil, e
	}
	out := make([]View, 0, len(rows))
	for i := range rows {
		out = append(out, view(&rows[i]))
	}
	return out, nil
}
func (s *Service) message(row *m.Installation) event_bus.TaskMessage {
	raw, _ := json.Marshal(map[string]string{"tenant_uuid": row.TenantUUID, "task_id": row.UUID.String()})
	return event_bus.TaskMessage{ID: row.UUID.String(), TenantKey: queueTenant, SubscriberID: s.subscriber, Topic: "powerx.plugin.install.v1", Payload: raw, Metadata: map[string]string{"taskbus_record_tenant": row.TenantUUID, "taskbus_record_id": row.UUID.String(), "kind": "plugin_install", "max_attempts": "10"}}
}
func (s *Service) dispatch(row *m.Installation) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if s.opts.Queue.Enqueue(ctx, s.message(row)) != nil {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	s.changed(ctx, row)
}
