package agent_session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	service "github.com/ArtisanCloud/PowerX/internal/service/agent_session"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct{ svc *service.Service }

func NewHandler(svc *service.Service) *Handler { return &Handler{svc: svc} }

// Register mounts one UUID-only service contract; no human-session aliases.
func (h *Handler) Register(parent *gin.RouterGroup) {
	g := parent.Group("/tenant/agent/sessions")
	g.Use(h.guard)
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:session_uuid", h.get)
	g.PATCH("/:session_uuid", h.rename)
	g.POST("/:session_uuid/archive", h.archive)
	g.DELETE("/:session_uuid", h.remove)
	g.POST("/:session_uuid/messages", h.appendMessage)
	g.GET("/:session_uuid/messages", h.messages)
	g.POST("/:session_uuid/invocations", h.invoke)
	g.GET("/:session_uuid/invocations/:invocation_uuid", h.invocation)
	g.POST("/:session_uuid/invocations/:invocation_uuid/cancel", h.cancel)
	g.GET("/:session_uuid/invocations/:invocation_uuid/events", h.events)
}

func respondError(c *gin.Context, err error) {
	status, code := http.StatusServiceUnavailable, service.ErrDependency.Error()
	for _, candidate := range []struct {
		err    error
		status int
	}{
		{service.ErrInvalid, 400}, {service.ErrUnauthorized, 401}, {service.ErrForbidden, 403},
		{service.ErrNotFound, 404}, {service.ErrConflict, 409}, {service.ErrExpired, 409}, {service.ErrContextExpired, 409}, {service.ErrEventCursorExpired, 409},
	} {
		if errors.Is(err, candidate.err) {
			status, code = candidate.status, candidate.err.Error()
			break
		}
	}
	message := dto.AgentSessionErrorMessage(c.GetHeader("Accept-Language"), code)
	dto.ResponseError(c, status, message, dto.NewErrorWithCode(status, code, message, nil))
	c.Abort()
}

func (h *Handler) guard(c *gin.Context) {
	if h.svc == nil {
		respondError(c, service.ErrDependency)
		return
	}
	if err := h.svc.CheckAccess(c.Request.Context()); err != nil {
		respondError(c, err)
		return
	}
	for key := range c.Request.Header {
		normalized := strings.ToLower(strings.ReplaceAll(key, "_", "-"))
		switch normalized {
		case "tenant-uuid", "x-tenant-uuid", "x-tenant-id", "x-powerx-tenant-uuid", "plugin-id", "x-plugin-id", "x-powerx-plugin-id", "service-actor", "x-service-actor":
			respondError(c, service.ErrInvalid)
			return
		}
	}
	query, err := c.Request.URL.Query(), error(nil)
	paged := c.Request.Method == http.MethodGet && (c.FullPath() == "/api/v1/tenant/agent/sessions" || c.FullPath() == "/api/v1/tenant/agent/sessions/:session_uuid/messages")
	for key, values := range query {
		if !paged || (key != "page" && key != "page_size") || len(values) != 1 {
			err = service.ErrInvalid
		}
	}
	if err != nil {
		respondError(c, err)
		return
	}
	for _, name := range []string{"session_uuid", "invocation_uuid"} {
		if raw := c.Param(name); raw != "" {
			value, err := uuid.Parse(raw)
			if err != nil || value == uuid.Nil || value.String() != raw {
				respondError(c, service.ErrInvalid)
				return
			}
		}
	}
	ctx := context.WithValue(c.Request.Context(), "authorization", c.GetHeader("Authorization"))
	c.Request = c.Request.WithContext(ctx)
	c.Next()
}

func bind(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128*1024)
	d := json.NewDecoder(c.Request.Body)
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		respondError(c, service.ErrInvalid)
		return false
	}
	allowed := map[string]bool{}
	typ := reflect.TypeOf(target).Elem()
	for i := 0; i < typ.NumField(); i++ {
		allowed[typ.Field(i).Tag.Get("json")] = true
	}
	values := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		_, duplicate := values[key]
		if err != nil || !ok || !allowed[key] || duplicate {
			respondError(c, service.ErrInvalid)
			return false
		}
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil || string(raw) == "null" {
			respondError(c, service.ErrInvalid)
			return false
		}
		values[key] = raw
	}
	if _, err := d.Token(); err != nil {
		respondError(c, service.ErrInvalid)
		return false
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		respondError(c, service.ErrInvalid)
		return false
	}
	raw, err := json.Marshal(values)
	if err != nil || json.Unmarshal(raw, target) != nil {
		respondError(c, service.ErrInvalid)
		return false
	}
	return true
}
func emptyBody(c *gin.Context) bool {
	if c.Request.Body == nil {
		return true
	}
	bytes, err := io.ReadAll(io.LimitReader(c.Request.Body, 2))
	if err != nil || len(bytes) != 0 {
		respondError(c, service.ErrInvalid)
		return false
	}
	return true
}
func id(c *gin.Context, name string) uuid.UUID { return uuid.MustParse(c.Param(name)) }
func reply(c *gin.Context, status int, data any, err error) {
	if err != nil {
		respondError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, status, data)
}
func pagination(c *gin.Context) (int, int, bool) {
	page, e1 := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, e2 := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if e1 != nil || e2 != nil || page < 1 || page > 1000000 || size < 1 || size > 100 {
		respondError(c, service.ErrInvalid)
		return 0, 0, false
	}
	return page, size, true
}
func (h *Handler) create(c *gin.Context) {
	var req struct {
		AgentUUID string `json:"agent_uuid"`
		Title     string `json:"title"`
	}
	if !bind(c, &req) {
		return
	}
	agent, err := uuid.Parse(req.AgentUUID)
	if err != nil || agent == uuid.Nil || agent.String() != req.AgentUUID {
		respondError(c, service.ErrInvalid)
		return
	}
	result, err := h.svc.Create(c.Request.Context(), agent, req.Title)
	reply(c, 201, result, err)
}
func (h *Handler) get(c *gin.Context) {
	if !emptyBody(c) {
		return
	}
	result, err := h.svc.Get(c.Request.Context(), id(c, "session_uuid"))
	reply(c, 200, result, err)
}
func (h *Handler) list(c *gin.Context) {
	if !emptyBody(c) {
		return
	}
	page, size, ok := pagination(c)
	if !ok {
		return
	}
	items, total, err := h.svc.List(c.Request.Context(), page, size)
	reply(c, 200, gin.H{"items": items, "total": total, "page": page, "page_size": size}, err)
}
func (h *Handler) rename(c *gin.Context) {
	var req struct {
		Title *string `json:"title"`
	}
	if !bind(c, &req) {
		return
	}
	if req.Title == nil {
		respondError(c, service.ErrInvalid)
		return
	}
	result, err := h.svc.Mutate(c.Request.Context(), id(c, "session_uuid"), "rename", *req.Title)
	reply(c, 200, result, err)
}
func (h *Handler) archive(c *gin.Context) { h.mutate(c, "archive") }
func (h *Handler) remove(c *gin.Context)  { h.mutate(c, "delete") }
func (h *Handler) mutate(c *gin.Context, operation string) {
	if !emptyBody(c) {
		return
	}
	result, err := h.svc.Mutate(c.Request.Context(), id(c, "session_uuid"), operation, "")
	reply(c, 200, result, err)
}
func (h *Handler) appendMessage(c *gin.Context) {
	var req struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if !bind(c, &req) {
		return
	}
	result, err := h.svc.Append(c.Request.Context(), id(c, "session_uuid"), c.GetHeader("Idempotency-Key"), req.Role, req.Content)
	reply(c, 201, result, err)
}
func (h *Handler) messages(c *gin.Context) {
	if !emptyBody(c) {
		return
	}
	page, size, ok := pagination(c)
	if !ok {
		return
	}
	items, total, err := h.svc.Messages(c.Request.Context(), id(c, "session_uuid"), page, size)
	reply(c, 200, gin.H{"items": items, "total": total, "page": page, "page_size": size}, err)
}
func (h *Handler) invoke(c *gin.Context) {
	var req struct {
		MessageUUID string `json:"message_uuid"`
	}
	if !bind(c, &req) {
		return
	}
	message, err := uuid.Parse(req.MessageUUID)
	if err != nil || message == uuid.Nil || message.String() != req.MessageUUID {
		respondError(c, service.ErrInvalid)
		return
	}
	result, err := h.svc.Invoke(c.Request.Context(), id(c, "session_uuid"), message, c.GetHeader("Idempotency-Key"))
	reply(c, 202, result, err)
}
func (h *Handler) invocation(c *gin.Context) {
	if !emptyBody(c) {
		return
	}
	result, err := h.svc.GetInvocation(c.Request.Context(), id(c, "session_uuid"), id(c, "invocation_uuid"))
	reply(c, 200, result, err)
}
func (h *Handler) cancel(c *gin.Context) {
	if !emptyBody(c) {
		return
	}
	result, err := h.svc.Cancel(c.Request.Context(), id(c, "session_uuid"), id(c, "invocation_uuid"))
	reply(c, 202, result, err)
}

// This is a state/final-result stream, not a token stream. Reconnection reads
// the same durable invocation and never calls Invoke. Disconnect only closes
// this subscription; cancellation uses the explicit cancel operation.
func (h *Handler) events(c *gin.Context) {
	if !emptyBody(c) {
		return
	}
	if h.svc.HasRunEvents() {
		h.runEvents(c)
		return
	}
	sessionID, runID := id(c, "session_uuid"), id(c, "invocation_uuid")
	cursor, err := eventCursor(c.GetHeader("Last-Event-ID"))
	if err != nil {
		respondError(c, err)
		return
	}
	run, err := h.svc.GetInvocation(c.Request.Context(), sessionID, runID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if cursor < 1 {
			writeSessionEvent(c, 1, "state", run)
			cursor = 1
		}
		switch run.Status {
		case "succeeded":
			if cursor < 2 {
				writeSessionEvent(c, 2, "final", run)
				cursor = 2
			}
			if cursor < 3 {
				writeSessionEvent(c, 3, "end", gin.H{"status": run.Status})
			}
			c.Writer.Flush()
			return
		case "failed", "cancelled":
			if cursor < 2 {
				writeSessionEvent(c, 2, "error", gin.H{"error_code": run.ReasonCode, "reason_code": run.ReasonCode})
				cursor = 2
			}
			if cursor < 3 {
				writeSessionEvent(c, 3, "end", gin.H{"status": run.Status})
			}
			c.Writer.Flush()
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
		run, err = h.svc.GetInvocation(c.Request.Context(), sessionID, runID)
		if err != nil {
			code := service.ErrDependency.Error()
			for _, known := range []error{service.ErrUnauthorized, service.ErrForbidden, service.ErrNotFound} {
				if errors.Is(err, known) {
					code = known.Error()
				}
			}
			if cursor < 2 {
				writeSessionEvent(c, 2, "error", gin.H{"error_code": code, "reason_code": code})
			}
			writeSessionEvent(c, 3, "end", gin.H{"status": "failed"})
			c.Writer.Flush()
			return
		}
	}
}

// runEvents only subscribes to an already admitted Run. The decimal cursor is
// Redis event_seq, so reconnecting cannot create a second execution.
func (h *Handler) runEvents(c *gin.Context) {
	var cursor uint64
	if raw := c.GetHeader("Last-Event-ID"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || strconv.FormatUint(value, 10) != raw {
			respondError(c, service.ErrEventCursorExpired)
			return
		}
		cursor = value
	}
	sessionID, runID := id(c, "session_uuid"), id(c, "invocation_uuid")
	subscription, err := h.svc.OpenRunSubscription(c.Request.Context(), sessionID, runID)
	if err != nil {
		respondError(c, err)
		return
	}
	lastAuth := time.Now()
	run, events, err := subscription.Read(c.Request.Context(), cursor, 100)
	if errors.Is(err, service.ErrEventCursorExpired) && cursor > run.EventSeq {
		respondError(c, err)
		return
	}
	if err != nil && !errors.Is(err, service.ErrEventCursorExpired) {
		respondError(c, err)
		return
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if errors.Is(err, service.ErrEventCursorExpired) {
			snapshot, snapshotErr := subscription.SnapshotState(c.Request.Context())
			if snapshotErr != nil {
				return
			}
			run = snapshot.Snapshot
			writeRunEvent(c, run.EventSeq, "agent_run.snapshot", snapshot)
			cursor = run.EventSeq
		} else {
			for _, event := range events {
				writeRunEvent(c, event.Seq, event.Type, event)
				cursor = event.Seq
			}
		}
		c.Writer.Flush()
		if durableTerminal(run.Status) && cursor >= run.EventSeq {
			return
		}
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
		if time.Since(lastAuth) >= 30*time.Second {
			subscription, err = h.svc.OpenRunSubscription(c.Request.Context(), sessionID, runID)
			if err != nil {
				return
			}
			lastAuth = time.Now()
		}
		run, events, err = subscription.Read(c.Request.Context(), cursor, 100)
		if err != nil && !errors.Is(err, service.ErrEventCursorExpired) {
			// The stream cannot change its HTTP status after headers are sent.
			// Closing it makes the client retry from the last acknowledged seq.
			return
		}
	}
}

func durableTerminal(status string) bool {
	switch status {
	case "completed", "partial", "needs_input", "blocked", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func writeRunEvent(c *gin.Context, seq uint64, event string, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", seq, event, payload)
}

// Event frames form one immutable invocation timeline: state=1, terminal=2,
// end=3. Last-Event-ID is therefore a durable acknowledgement cursor rather
// than a second execution input; reconnect never invokes the agent again.
func eventCursor(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	cursor, err := strconv.Atoi(value)
	if err != nil || cursor < 0 || cursor > 3 || strconv.Itoa(cursor) != value {
		return 0, service.ErrEventCursorExpired
	}
	return cursor, nil
}

func writeSessionEvent(c *gin.Context, id int, event string, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(c.Writer, "id: %d\nevent: %s\ndata: %s\n\n", id, event, payload)
}
