package capability_registry

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
)

type InvocationStream struct {
	Body    io.ReadCloser
	TraceID string
}

// OpenStream uses precisely the same Selector policy and authorizer as Invoke.
func (s *Selector) OpenStream(ctx context.Context, req CapabilityInvokeRequest, authorization string) (*InvocationStream, error) {
	if s == nil || s.invoker == nil {
		return nil, ErrSelectorUnavailable
	}
	invoker, ok := s.invoker.(interface {
		OpenStream(context.Context, InvocationInput, string) (*InvocationStream, error)
	})
	if !ok {
		return nil, pluginServiceError(503, "CAPABILITY_SERVICE_STREAM_UNAVAILABLE")
	}
	var stream *InvocationStream
	_, err := s.invoke(ctx, req, func(ctx context.Context, in InvocationInput) (InvocationResult, error) {
		var err error
		stream, err = invoker.OpenStream(ctx, in, authorization)
		return InvocationResult{}, err
	})
	return stream, err
}

// OpenStream supports only declared Core REST stream bindings. Plugin service
// tickets require a separate streaming contract and cannot fall back to proxying.
func (s *InvocationService) OpenStream(ctx context.Context, in InvocationInput, authorization string) (_ *InvocationStream, err error) {
	if strings.TrimSpace(in.TraceID) == "" {
		in.TraceID = uuid.NewString()
	}
	ctx, record, err := s.prepareInvocation(ctx, in)
	if err != nil {
		return nil, err
	}
	started := s.now()
	protocolUsed := "rest"
	audit := func(status, summary string) {
		if s.audit != nil {
			auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			s.audit.RecordInvocation(auditCtx, InvocationAuditInput{TraceID: in.TraceID, TenantUUID: in.TenantUUID,
				PluginID: record.PluginID, CapabilityID: in.CapabilityID, PreferredProtocol: in.PreferredProtocol,
				ProtocolUsed: protocolUsed, Status: status, IdempotencyKey: in.IdempotencyKey,
				RequestPayload: map[string]interface{}{"stream": true}, ResponsePayload: map[string]interface{}{"stream": true},
				ErrorSummary: summary, Latency: s.now().Sub(started)})
		}
	}
	defer func() {
		if err != nil {
			audit("failed", err.Error())
		}
	}()
	if record.PluginID != "corex.platform" || hasServiceTicketBinding(record) || !strings.HasSuffix(record.CapabilityID, ".stream") {
		return nil, pluginServiceError(400, "CAPABILITY_SERVICE_STREAM_UNSUPPORTED")
	}
	if claims := reqctx.GetClaims(ctx); claims == nil {
		return nil, &DirectGrantError{401}
	} else if claims.Issuer == "powerx-sts" && reqctx.STSIssuanceStatus(ctx) != 0 {
		return nil, &DirectGrantError{reqctx.STSIssuanceStatus(ctx)}
	}
	// User and service streams both require an active tenant registration.
	facts, err := s.catalog.credentialAccess.publishedAndRegistered(ctx, in.TenantUUID, []string{in.CapabilityID})
	if err != nil {
		return nil, &DirectGrantError{503}
	}
	if !facts.published[in.CapabilityID] || !facts.registered[in.CapabilityID] {
		return nil, &DirectGrantError{403}
	}
	raw, err := json.Marshal(in.Payload)
	if err != nil {
		return nil, pluginServiceError(400, "CAPABILITY_SERVICE_STREAM_PAYLOAD_INVALID")
	}
	route, err := s.router.Invoke(ctx, router.InvokeRequest{CapabilityID: in.CapabilityID, TenantUUID: in.TenantUUID,
		Payload: raw, StickyKey: extractStickyKey(in.Context), PreferredProtocol: in.PreferredProtocol})
	if err != nil {
		return nil, err
	}
	if route.FallbackUsed {
		return nil, pluginServiceError(503, "CAPABILITY_SERVICE_STREAM_FALLBACK_FORBIDDEN")
	}
	if route.Transport == "core_internal" {
		streamer, ok := s.coreInvoker.(CoreCapabilityStreamInvoker)
		if !ok {
			return nil, pluginServiceError(503, "CAPABILITY_CORE_STREAM_UNAVAILABLE")
		}
		var declared bool
		var bindings []models.ProtocolBinding
		if json.Unmarshal(record.Protocols, &bindings) == nil {
			for _, b := range bindings {
				if b.Channel == "core_internal" && b.Method == "INVOKE" && b.Endpoint == route.Endpoint && strings.HasPrefix(b.Endpoint, "core://") {
					declared = true
				}
			}
		}
		if !declared {
			return nil, pluginServiceError(400, "CAPABILITY_SERVICE_STREAM_BINDING_INVALID")
		}
		body, ok := toStringAnyMap(in.Payload["body"])
		if !ok {
			return nil, pluginServiceError(400, "CAPABILITY_SERVICE_STREAM_PAYLOAD_INVALID")
		}
		streamBody, e := streamer.InvokeCoreCapabilityStream(reqctx.WithTraceID(ctx, in.TraceID), CoreCapabilityInvokeInput{CapabilityID: in.CapabilityID, TenantUUID: in.TenantUUID, Method: "INVOKE", Endpoint: route.Endpoint, Body: body, Payload: in.Payload, Context: in.Context, Query: mapStringString(in.Payload["query"])})
		if e != nil {
			return nil, e
		}
		if streamBody == nil {
			return nil, pluginServiceError(503, "CAPABILITY_CORE_STREAM_UNAVAILABLE")
		}
		protocolUsed = "core_internal"
		return &InvocationStream{TraceID: in.TraceID, Body: &auditedStreamBody{ReadCloser: streamBody, finish: audit}}, nil
	}
	binding, err := coreStreamBinding(record, route)
	if err != nil {
		return nil, err
	}
	body, query, err := streamBusinessPayload(in, binding)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(s.httpBaseURL)
	if err != nil || base.Host == "" || base.User != nil || (base.Scheme != "http" && base.Scheme != "https") || (base.Path != "" && base.Path != "/") || base.RawQuery != "" || base.Fragment != "" {
		return nil, pluginServiceError(503, "CAPABILITY_SERVICE_STREAM_ORIGIN_UNAVAILABLE")
	}
	base.Path, base.RawQuery = binding.Endpoint, query.Encode()
	request, err := http.NewRequestWithContext(ctx, binding.Method, base.String(), bytes.NewReader(body))
	if err != nil {
		return nil, pluginServiceError(400, "CAPABILITY_SERVICE_STREAM_PAYLOAD_INVALID")
	}
	request.Header.Set("Authorization", authorization)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("X-Trace-ID", in.TraceID)
	client := *s.aiModalHTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return nil, pluginServiceError(502, "CAPABILITY_SERVICE_STREAM_TRANSPORT_FAILED")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024+1))
		if readErr != nil || len(raw) > 4*1024*1024 {
			return nil, pluginServiceError(502, "CAPABILITY_SERVICE_STREAM_RESPONSE_INVALID")
		}
		return nil, newPluginServiceUpstreamError(response.StatusCode, binding.Method, binding.Endpoint, raw)
	}
	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "text/event-stream" {
		response.Body.Close()
		return nil, pluginServiceError(502, "CAPABILITY_SERVICE_STREAM_RESPONSE_INVALID")
	}
	return &InvocationStream{TraceID: in.TraceID, Body: &auditedStreamBody{ReadCloser: response.Body, finish: audit}}, nil
}

func coreStreamBinding(record *models.CapabilityRecord, route router.InvokeResult) (models.ProtocolBinding, error) {
	var bindings []models.ProtocolBinding
	if json.Unmarshal(record.Protocols, &bindings) == nil {
		for _, binding := range bindings {
			if binding.Channel == "rest" && (route.Transport == "rest" || route.Transport == "http") && binding.Endpoint == route.Endpoint && binding.Method == "POST" {
				if _, err := FixedPluginServicePath(binding.Endpoint); err == nil {
					return binding, nil
				}
			}
		}
	}
	return models.ProtocolBinding{}, pluginServiceError(400, "CAPABILITY_SERVICE_STREAM_BINDING_INVALID")
}

func streamBusinessPayload(in InvocationInput, binding models.ProtocolBinding) ([]byte, url.Values, error) {
	invalid := pluginServiceError(400, "CAPABILITY_SERVICE_STREAM_PAYLOAD_INVALID")
	if len(in.Context) != 0 {
		return nil, nil, invalid
	}
	for key, value := range in.Payload {
		switch key {
		case "body", "query":
		case "method":
			if value != binding.Method {
				return nil, nil, invalid
			}
		case "endpoint":
			if value != binding.Endpoint {
				return nil, nil, invalid
			}
		case "headers":
			// Compatibility assertions only: these values are never forwarded.
			headers, ok := value.(map[string]interface{})
			if !ok {
				return nil, nil, invalid
			}
			for key, value := range headers {
				if !(strings.EqualFold(key, "Content-Type") && value == "application/json" || strings.EqualFold(key, "Accept") && value == "text/event-stream") {
					return nil, nil, invalid
				}
			}
		default:
			return nil, nil, invalid
		}
	}
	body, ok := in.Payload["body"].(map[string]interface{})
	if !ok {
		return nil, nil, invalid
	}
	query := url.Values{}
	if value, exists := in.Payload["query"]; exists {
		values, ok := value.(map[string]interface{})
		if !ok {
			return nil, nil, invalid
		}
		for key, value := range values {
			if key != "env" || (value != "dev" && value != "staging" && value != "prod") {
				return nil, nil, invalid
			}
			query.Set(key, value.(string))
		}
	}
	raw, err := json.Marshal(body)
	return raw, query, err
}

type auditedStreamBody struct {
	io.ReadCloser
	once   sync.Once
	finish func(string, string)
}

func (b *auditedStreamBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.once.Do(func() {
			if err == io.EOF {
				b.finish("completed", "")
			} else {
				b.finish("failed", "stream interrupted")
			}
		})
	}
	return n, err
}
func (b *auditedStreamBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { b.finish("failed", "stream closed before completion") })
	return err
}
