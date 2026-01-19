package knowledge_space

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	event_hotfix "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/event_hotfix"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// EventHandler exposes HTTP endpoints for event hotfix orchestration.
type EventHandler struct {
	svc *event_hotfix.Service

	signatureSecret string
	signatureHeader string
	timestampHeader string
	allowedSkew     time.Duration
}

func NewEventHandler(deps *shared.Deps) *EventHandler {
	if deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.EventHotfix == nil {
		return nil
	}
	header := strings.TrimSpace(os.Getenv("PX_KNOWLEDGE_EVENT_SIGNATURE_HEADER"))
	if header == "" {
		header = "X-PowerX-Signature"
	}
	tsHeader := strings.TrimSpace(os.Getenv("PX_KNOWLEDGE_EVENT_TIMESTAMP_HEADER"))
	if tsHeader == "" {
		tsHeader = "X-PowerX-Timestamp"
	}
	skewSec := 300
	if raw := strings.TrimSpace(os.Getenv("PX_KNOWLEDGE_EVENT_ALLOWED_SKEW_SECONDS")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			skewSec = v
		}
	}
	return &EventHandler{
		svc:             deps.KnowledgeSpace.EventHotfix,
		signatureSecret: strings.TrimSpace(os.Getenv("PX_KNOWLEDGE_EVENT_SIGNATURE_SECRET")),
		signatureHeader: header,
		timestampHeader: tsHeader,
		allowedSkew:     time.Duration(skewSec) * time.Second,
	}
}

func (h *EventHandler) Apply(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "event hotfix unavailable", nil)
		return
	}
	raw, _ := c.GetRawData()
	if !h.verifySignature(c, raw) {
		dto.ResponseError(c, http.StatusUnauthorized, "签名校验失败", nil)
		return
	}
	c.Request.Body = ioNopCloser(bytes.NewReader(raw))
	var req eventApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	result, err := h.svc.Apply(c.Request.Context(), req.toInput())
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *EventHandler) Retry(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "event hotfix unavailable", nil)
		return
	}
	raw, _ := c.GetRawData()
	if !h.verifySignature(c, raw) {
		dto.ResponseError(c, http.StatusUnauthorized, "签名校验失败", nil)
		return
	}
	c.Request.Body = ioNopCloser(bytes.NewReader(raw))
	var req eventApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	result, err := h.svc.Retry(c.Request.Context(), req.toInput())
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *EventHandler) HotUpdate(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "event hotfix unavailable", nil)
		return
	}
	raw, _ := c.GetRawData()
	if !h.verifySignature(c, raw) {
		dto.ResponseError(c, http.StatusUnauthorized, "签名校验失败", nil)
		return
	}
	c.Request.Body = ioNopCloser(bytes.NewReader(raw))
	var req struct {
		EventID  string         `json:"eventId"`
		SpaceID  string         `json:"spaceId" binding:"required,uuid4"`
		Payload  map[string]any `json:"payload"`
		RetryCount int          `json:"retryCount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	eventID := strings.TrimSpace(req.EventID)
	if eventID == "" {
		eventID = "hot-update:" + uuid.NewString()
	}
	payload := req.Payload
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["spaceId"] = strings.TrimSpace(req.SpaceID)
	result, err := h.svc.Apply(c.Request.Context(), event_hotfix.ApplyInput{
		EventID:    eventID,
		EventType:  "index.hot_update",
		Payload:    payload,
		ReceivedAt: time.Now().UTC(),
		RetryCount: req.RetryCount,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *EventHandler) RefreshAgent(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "event hotfix unavailable", nil)
		return
	}
	raw, _ := c.GetRawData()
	if !h.verifySignature(c, raw) {
		dto.ResponseError(c, http.StatusUnauthorized, "签名校验失败", nil)
		return
	}
	c.Request.Body = ioNopCloser(bytes.NewReader(raw))
	var req struct {
		EventID         string `json:"eventId"`
		TargetEventType string `json:"targetEventType"`
		RetryCount      int    `json:"retryCount"`
	}
	_ = c.ShouldBindJSON(&req)
	eventID := strings.TrimSpace(req.EventID)
	if eventID == "" {
		eventID = "agent-refresh:" + uuid.NewString()
	}
	payload := map[string]any{
		"targetEventType": strings.TrimSpace(req.TargetEventType),
	}
	result, err := h.svc.Apply(c.Request.Context(), event_hotfix.ApplyInput{
		EventID:    eventID,
		EventType:  "agent.weight.refresh",
		Payload:    payload,
		ReceivedAt: time.Now().UTC(),
		RetryCount: req.RetryCount,
	})
	if err != nil && !errors.Is(err, event_hotfix.ErrDuplicateEvent) {
		h.handleError(c, err)
		return
	}
	statusText := "ok"
	if err != nil {
		statusText = "duplicate"
	} else if result != nil {
		statusText = result.Status
	}
	dto.ResponseSuccess(c, gin.H{"status": statusText})
}

func (h *EventHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, event_hotfix.ErrInvalidEvent):
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
	case errors.Is(err, event_hotfix.ErrPolicyMissing):
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
	case errors.Is(err, event_hotfix.ErrDuplicateEvent):
		dto.ResponseError(c, http.StatusConflict, err.Error(), err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
	}
}

func (req eventApplyRequest) toInput() event_hotfix.ApplyInput {
	received := req.ReceivedAt
	if received == nil || received.IsZero() {
		now := time.Now()
		received = &now
	}
	payload := req.Payload
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["eventType"] = req.EventType
	return event_hotfix.ApplyInput{
		EventID:    strings.TrimSpace(req.EventID),
		EventType:  strings.TrimSpace(req.EventType),
		Payload:    payload,
		ReceivedAt: received.UTC(),
		RetryCount: req.RetryCount,
	}
}

func (h *EventHandler) verifySignature(c *gin.Context, rawBody []byte) bool {
	if h == nil || strings.TrimSpace(h.signatureSecret) == "" {
		return true
	}
	sig := strings.TrimSpace(c.GetHeader(h.signatureHeader))
	tsRaw := strings.TrimSpace(c.GetHeader(h.timestampHeader))
	if sig == "" || tsRaw == "" {
		return false
	}
	ts, ok := parseSignatureTimestamp(tsRaw)
	if !ok {
		return false
	}
	now := time.Now().UTC()
	if h.allowedSkew > 0 && now.Sub(ts) > h.allowedSkew {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.signatureSecret))
	_, _ = mac.Write([]byte(tsRaw))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(strings.ToLower(expected)), []byte(strings.ToLower(sig)))
}

func parseSignatureTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		// Support seconds or milliseconds.
		if i > 1_000_000_000_000 {
			return time.UnixMilli(i).UTC(), true
		}
		return time.Unix(i, 0).UTC(), true
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		return ts.UTC(), true
	}
	return time.Time{}, false
}

type nopCloser struct{ *bytes.Reader }

func ioNopCloser(r *bytes.Reader) *nopCloser { return &nopCloser{Reader: r} }

func (c *nopCloser) Close() error { return nil }
