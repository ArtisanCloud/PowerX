package runtime_host

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	cache "github.com/ArtisanCloud/PowerX/internal/infra/cache/runtime_host"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const RealtimeContract = "realtime.service.v1"
const RealtimeEndpoint = "core://runtime/realtime"
const RealtimePublish = "com.corex.runtime.realtime.service_publish"
const RealtimeStream = "com.corex.runtime.realtime.stream"

var realtimeCursorPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,19})-(0|[1-9][0-9]{0,19})$`)
var realtimeTopicPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,512}$`)

type RealtimeEvent struct {
	EventID       string `json:"event_id"`
	TenantUUID    string `json:"tenant_uuid"`
	Topic         string `json:"topic"`
	MemberUUID    string `json:"member_uuid,omitempty"`
	TraceID       string `json:"trace_id"`
	PayloadBase64 string `json:"payload_base64"`
}
type RealtimeOperation struct {
	ContractVersion string         `json:"contract_version"`
	TenantKey       string         `json:"tenant_key"`
	Namespace       string         `json:"namespace"`
	Operation       string         `json:"operation"`
	Cursor          *string        `json:"cursor,omitempty"`
	Event           *RealtimeEvent `json:"event,omitempty"`
}
type RealtimeInvoker struct {
	db    *gorm.DB
	store *cache.Store
}

func NewRealtimeInvoker(db *gorm.DB, service *Service) *RealtimeInvoker {
	if service == nil {
		return &RealtimeInvoker{db: db}
	}
	return &RealtimeInvoker{db, service.cache}
}
func (i *RealtimeInvoker) authorize(ctx context.Context, in cap.CoreCapabilityInvokeInput) (string, error) {
	if i == nil || i.db == nil || i.store == nil {
		return "", cacheInvokeError(503, "REALTIME_UNAVAILABLE")
	}
	if !cap.ServiceCredential(ctx) {
		return "", cacheInvokeError(401, "REALTIME_SERVICE_REQUIRED")
	}
	tenant, err := reqctx.CanonicalTenantUUID(reqctx.GetTenantUUID(ctx))
	if err != nil || tenant != in.TenantUUID {
		return "", cacheInvokeError(403, "REALTIME_SCOPE_FORBIDDEN")
	}
	access, err := cap.NewGrantStatusService(i.db).CurrentAccess(ctx)
	if err == nil {
		err = access.Require(in.CapabilityID)
	}
	if err != nil {
		var grant *cap.DirectGrantError
		if errors.As(err, &grant) {
			return "", cacheInvokeError(grant.HTTPStatus(), "REALTIME_GRANT_REQUIRED")
		}
		return "", cacheInvokeError(503, "REALTIME_UNAVAILABLE")
	}
	if access.Subject == "" {
		return "", cacheInvokeError(401, "REALTIME_SERVICE_REQUIRED")
	}
	return access.Subject, nil
}
func (i *RealtimeInvoker) operation(ctx context.Context, in cap.CoreCapabilityInvokeInput) (RealtimeOperation, string, error) {
	invalid := cacheInvokeError(400, "REALTIME_INVALID_ARGUMENT")
	var op RealtimeOperation
	if in.Endpoint != RealtimeEndpoint || in.Method != "INVOKE" || len(in.Query) > 0 || len(in.Context) > 0 {
		return op, "", invalid
	}
	for k := range in.Payload {
		if k != "body" {
			return op, "", invalid
		}
	}
	raw, err := json.Marshal(in.Body)
	if err != nil || len(raw) > 130<<10 {
		return op, "", invalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&op) != nil || d.Decode(new(any)) != io.EOF || op.ContractVersion != RealtimeContract || !textValid(op.Namespace, 128) || op.TenantKey != in.TenantUUID {
		return op, "", invalid
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	switch op.Operation {
	case "publish":
		if in.CapabilityID != RealtimePublish || op.Event == nil || fields["cursor"] != nil {
			return op, "", invalid
		}
		e := op.Event
		if id, err := uuid.Parse(e.EventID); err != nil || id.String() != e.EventID || e.TenantUUID != op.TenantKey || !realtimeTopicPattern.MatchString(e.Topic) || !validRealtimePayload(e.PayloadBase64) || len(e.TraceID) > 128 || strings.ContainsAny(e.TraceID, "\r\n\x00") {
			return op, "", invalid
		}
		if e.MemberUUID != "" {
			if id, err := uuid.Parse(e.MemberUUID); err != nil || id.String() != e.MemberUUID {
				return op, "", invalid
			}
		}
	case "cursor":
		if in.CapabilityID != RealtimeStream || fields["event"] != nil || fields["cursor"] != nil {
			return op, "", invalid
		}
	case "subscribe":
		if in.CapabilityID != RealtimeStream || fields["event"] != nil || op.Cursor == nil || !realtimeCursorPattern.MatchString(*op.Cursor) {
			return op, "", invalid
		}
	default:
		return op, "", invalid
	}
	subject, err := i.authorize(ctx, in)
	return op, subject, err
}
func (i *RealtimeInvoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]any, error) {
	if in.CapabilityID != RealtimePublish && in.CapabilityID != RealtimeStream {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	op, subject, err := i.operation(ctx, in)
	if err != nil {
		return nil, err
	}
	var cursor string
	switch op.Operation {
	case "publish":
		raw, _ := json.Marshal(op.Event)
		cursor, err = i.store.AppendRealtime(ctx, op.TenantKey, subject, op.Namespace, raw)
	case "cursor":
		cursor, err = i.store.RealtimeCursor(ctx, op.TenantKey, subject, op.Namespace)
	default:
		return nil, cacheInvokeError(400, "REALTIME_STREAM_REQUIRED")
	}
	if err != nil {
		return nil, cacheInvokeError(503, "REALTIME_UNAVAILABLE")
	}
	return map[string]any{"contract_version": RealtimeContract, "operation": op.Operation, "tenant_key": op.TenantKey, "namespace": op.Namespace, "cursor": cursor}, nil
}
func (i *RealtimeInvoker) InvokeCoreCapabilityStream(ctx context.Context, in cap.CoreCapabilityInvokeInput) (io.ReadCloser, error) {
	if in.CapabilityID != RealtimeStream {
		return nil, cap.ErrCoreCapabilityNotHandled
	}
	op, subject, err := i.operation(ctx, in)
	if err != nil {
		return nil, err
	}
	if op.Operation != "subscribe" {
		return nil, cacheInvokeError(400, "REALTIME_INVALID_ARGUMENT")
	}
	reader, writer := io.Pipe()
	run, cancel := context.WithCancel(ctx)
	context.AfterFunc(run, func() { _ = writer.CloseWithError(run.Err()) })
	go func() {
		defer cancel()
		defer writer.Close()
		cursor := *op.Cursor
		end := time.Now().Add(30 * time.Second)
		heartbeat := time.Now().Add(5 * time.Second)
		send := func(kind string, payload any) error {
			data, e := json.Marshal(payload)
			if e != nil {
				return e
			}
			_, e = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", kind, data)
			return e
		}
		if send("ready", map[string]any{"contract_version": RealtimeContract, "namespace": op.Namespace, "tenant_key": op.TenantKey, "cursor": cursor}) != nil {
			return
		}
		for run.Err() == nil {
			if _, e := i.authorize(run, in); e != nil {
				_ = send("error", map[string]string{"code": "REALTIME_GRANT_REQUIRED"})
				return
			}
			if time.Now().After(end) {
				_ = send("reconnect", map[string]string{"cursor": cursor})
				return
			}
			entries, e := i.store.ReadRealtime(run, op.TenantKey, subject, op.Namespace, cursor)
			if e != nil {
				code := "REALTIME_UNAVAILABLE"
				if errors.Is(e, cache.ErrRealtimeCursorExpired) {
					code = "REALTIME_CURSOR_EXPIRED"
				}
				_ = send("error", map[string]string{"code": code})
				return
			}
			if len(entries) == 0 && time.Now().After(heartbeat) {
				if send("heartbeat", map[string]any{"contract_version": RealtimeContract, "namespace": op.Namespace, "tenant_key": op.TenantKey, "cursor": cursor}) != nil {
					return
				}
				heartbeat = time.Now().Add(5 * time.Second)
			}
			if _, e := i.authorize(run, in); e != nil {
				_ = send("error", map[string]string{"code": "REALTIME_GRANT_REQUIRED"})
				return
			}
			for _, entry := range entries {
				var event RealtimeEvent
				if json.Unmarshal(entry.Value, &event) != nil || event.TenantUUID != op.TenantKey {
					_ = send("error", map[string]string{"code": "REALTIME_ENTRY_INVALID"})
					return
				}
				if send("event", map[string]any{"contract_version": RealtimeContract, "namespace": op.Namespace, "cursor": entry.Cursor, "event": event}) != nil {
					return
				}
				cursor = entry.Cursor
			}
		}
	}()
	return &realtimeReader{ReadCloser: reader, cancel: cancel}, nil
}

type realtimeReader struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *realtimeReader) Close() error { r.cancel(); return r.ReadCloser.Close() }

func validRealtimePayload(encoded string) bool {
	raw, err := base64.StdEncoding.Strict().DecodeString(encoded)
	return err == nil && base64.StdEncoding.EncodeToString(raw) == encoded && len(raw) <= 64<<10 && json.Valid(raw)
}
