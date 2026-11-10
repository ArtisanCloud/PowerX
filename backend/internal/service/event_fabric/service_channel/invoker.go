package servicechannel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

type Invoker struct{ Service *Service }

func Decode(body any) (Request, error) {
	var in Request
	raw, err := json.Marshal(body)
	if err != nil || len(raw) > 2<<20 {
		return in, failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF {
		return in, failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
	}
	return in, nil
}
func (i *Invoker) input(in cap.CoreCapabilityInvokeInput) (Request, error) {
	if in.CapabilityID != PublishCapability && in.CapabilityID != ConsumeCapability {
		return Request{}, cap.ErrCoreCapabilityNotHandled
	}
	if in.Endpoint != Endpoint || in.Method != "INVOKE" || len(in.Query) > 0 || len(in.Context) > 0 {
		return Request{}, failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
	}
	for key := range in.Payload {
		if key != "body" {
			return Request{}, failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
		}
	}
	op, err := Decode(in.Body)
	if err != nil {
		return op, err
	}
	if op.TenantUUID != in.TenantUUID {
		return op, failure(403, "EVENT_FABRIC_TENANT_FORBIDDEN")
	}
	if op.Operation != "resolve_topics" && (op.Operation == "publish") != (in.CapabilityID == PublishCapability) {
		return op, failure(403, "EVENT_FABRIC_OPERATION_FORBIDDEN")
	}
	return op, nil
}
func (i *Invoker) InvokeCoreCapability(ctx context.Context, in cap.CoreCapabilityInvokeInput) (map[string]any, error) {
	op, err := i.input(in)
	if err != nil {
		return nil, err
	}
	switch op.Operation {
	case "resolve_topics":
		return i.Service.ResolveTopics(ctx, op, in.CapabilityID)
	case "publish":
		return i.Service.Publish(ctx, op)
	case "ack", "nack":
		return i.Service.Settle(ctx, op)
	case "subscribe":
		return nil, failure(400, "EVENT_FABRIC_STREAM_REQUIRED")
	default:
		return nil, failure(400, "EVENT_FABRIC_INVALID_ARGUMENT")
	}
}

// Stream 只推送已持久化的投递；网络写入成功不代表业务完成，必须另行 ACK。
// 每次发送前重新校验凭据、grant、registration 和 topic ACL；断流由数据库期限恢复。
func (s *Service) Stream(ctx context.Context, in Request, send func(string, any) error) (err error) {
	start := time.Now()
	defer func() { s.record(ctx, in, nil, err, start) }()
	if err := s.ValidateSubscription(ctx, in); err != nil {
		return err
	}
	if err := send("ready", map[string]any{"contract_version": Contract, "tenant_uuid": in.TenantUUID, "subscriber_id": in.SubscriberID}); err != nil {
		return err
	}
	end := time.NewTimer(30 * time.Second)
	defer end.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		items, err := s.Poll(ctx, in)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := s.ValidateSubscription(ctx, in); err != nil {
				return err
			}
			if !s.clock().UTC().Before(item.AckDeadline) {
				continue
			}
			started := time.Now()
			sendErr := send("delivery", item)
			auditRequest := in
			auditRequest.Operation = "deliver"
			auditRequest.DeliveryID = item.DeliveryID
			auditRequest.Event = &Event{EventID: item.EventID, Topic: item.Topic}
			s.record(ctx, auditRequest, map[string]any{"trace_id": item.TraceID}, sendErr, started)
			if sendErr != nil {
				return sendErr
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-end.C:
			return send("reconnect", map[string]string{"contract_version": Contract})
		case <-tick.C:
		}
	}
}
func (i *Invoker) InvokeCoreCapabilityStream(ctx context.Context, in cap.CoreCapabilityInvokeInput) (io.ReadCloser, error) {
	op, err := i.input(in)
	if err != nil {
		return nil, err
	}
	if err = i.Service.ValidateSubscription(ctx, op); err != nil {
		return nil, err
	}
	r, w := io.Pipe()
	run, cancel := context.WithCancel(ctx)
	context.AfterFunc(run, func() { _ = w.CloseWithError(run.Err()) })
	go func() {
		defer cancel()
		defer w.Close()
		send := func(kind string, v any) error {
			data, err := json.Marshal(v)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data)
			return err
		}
		if err := i.Service.Stream(run, op, send); err != nil && run.Err() == nil {
			code := dto.CodeOf(err)
			if code == "" {
				code = "EVENT_FABRIC_UNAVAILABLE"
			}
			_ = send("error", map[string]string{"code": code})
		}
	}()
	return &streamReader{ReadCloser: r, cancel: cancel}, nil
}

type streamReader struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *streamReader) Close() error { r.cancel(); return r.ReadCloser.Close() }
