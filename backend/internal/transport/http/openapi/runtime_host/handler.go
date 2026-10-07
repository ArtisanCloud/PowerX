package runtime_host

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	svc "github.com/ArtisanCloud/PowerX/internal/service/runtime_host"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/runtime_host"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

//go:embed locales/messages.json
var localeJSON []byte
var locales = func() map[string]map[string]string {
	var out map[string]map[string]string
	if e := json.Unmarshal(localeJSON, &out); e != nil {
		panic(e)
	}
	return out
}()

type Handler struct{ svc *svc.Service }

func NewHandler(s *svc.Service) *Handler { return &Handler{svc: s} }
func message(c *gin.Context, key string) string {
	lang := "zh-CN"
	if strings.HasPrefix(strings.ToLower(c.GetHeader("Accept-Language")), "en") {
		lang = "en-US"
	}
	return locales[lang][key]
}
func (h *Handler) respond(c *gin.Context, module string, data any, err error) {
	if err == nil {
		c.JSON(200, dto.SuccessResponse{Code: 200, Message: message(c, "success"), Data: data, Timestamp: time.Now().Unix(), RequestID: repo.RequestID(c.Request.Context())})
		return
	}
	kind := "upstream_dependency"
	var typed *svc.Error
	if errors.As(err, &typed) {
		kind = typed.Kind
	}
	status := map[string]int{"invalid_argument": 400, "unauthorized": 401, "forbidden": 403, "not_found": 404, "conflict": 409, "upstream_dependency": 503}[kind]
	code := module + "_" + strings.ToUpper(kind)
	logger.WarnF(logger.WithLogFields(c.Request.Context(), map[string]interface{}{"module": "runtime_host", "reason_code": code, "request_id": repo.RequestID(c.Request.Context()), "method": c.Request.Method, "path": c.FullPath()}), "runtime_host.request_failed")
	dto.RespondErrorFrom(c, dto.NewErrorWithCode(status, code, message(c, kind), nil))
}
func (h *Handler) context(c *gin.Context, module string, query bool) (context.Context, bool) {
	requestID := c.GetString("request_id")
	if requestID == "" {
		requestID = uuid.NewString()
		c.Set("request_id", requestID)
	}
	ctx := repo.WithRequestID(c.Request.Context(), requestID)
	c.Request = c.Request.WithContext(ctx)
	c.Header("X-Request-ID", requestID)
	values, e := urlQuery(c)
	if e != nil {
		h.respond(c, module, nil, e)
		return ctx, false
	}
	for name, v := range values {
		if !query || (name != "namespace" && name != "key") || len(v) != 1 {
			h.respond(c, module, nil, svc.Invalid())
			return ctx, false
		}
	}
	for name := range c.Request.Header {
		normalized := strings.ToLower(strings.ReplaceAll(name, "_", "-"))
		if normalized == "tenant-uuid" || normalized == "x-tenant-uuid" || normalized == "tenant-id" || normalized == "x-tenant-id" || normalized == "x-plugin-id" || normalized == "x-caller-subject" {
			h.respond(c, module, nil, svc.Invalid())
			return ctx, false
		}
	}
	return ctx, true
}
func decode(c *gin.Context, target any, required []string, optional ...string) error {
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		return svc.Invalid()
	}
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return svc.Invalid()
	}
	allowed := map[string]bool{}
	for _, key := range append(required, optional...) {
		allowed[key] = true
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, e = d.Token()
		if e != nil {
			return svc.Invalid()
		}
		key, ok := token.(string)
		if !ok || !allowed[key] || fields[key] != nil {
			return svc.Invalid()
		}
		var raw json.RawMessage
		if e = d.Decode(&raw); e != nil {
			return svc.Invalid()
		}
		if !utf8.Valid(raw) {
			return svc.Invalid()
		}
		// payload/result 是 JSON 值，允许显式 JSON null；控制字段不允许 null。
		if string(raw) == "null" && key != "payload" && key != "result" {
			return svc.Invalid()
		}
		fields[key] = raw
	}
	if _, e = d.Token(); e != nil {
		return svc.Invalid()
	}
	if _, e = d.Token(); e != io.EOF {
		return svc.Invalid()
	}
	for _, key := range required {
		if fields[key] == nil {
			return svc.Invalid()
		}
	}
	raw, e := json.Marshal(fields)
	if e != nil {
		return svc.Invalid()
	}
	if e = json.Unmarshal(raw, target); e != nil {
		return svc.Invalid()
	}
	return nil
}
func emptyBody(c *gin.Context) error {
	var b [1]byte
	n, e := c.Request.Body.Read(b[:])
	if n != 0 || e != io.EOF {
		return svc.Invalid()
	}
	return nil
}
func (h *Handler) GetCache(c *gin.Context) {
	ctx, ok := h.context(c, "CACHE", true)
	if !ok {
		return
	}
	if e := emptyBody(c); e != nil {
		h.respond(c, "CACHE", nil, e)
		return
	}
	out, e := h.svc.GetCache(ctx, c.Query("namespace"), c.Query("key"))
	h.respond(c, "CACHE", out, e)
}
func (h *Handler) SetCache(c *gin.Context) {
	ctx, ok := h.context(c, "CACHE", false)
	if !ok {
		return
	}
	var in struct {
		Namespace string `json:"namespace"`
		Key       string `json:"key"`
		Value     string `json:"value_base64"`
		TTL       int64  `json:"ttl_ms"`
	}
	e := decode(c, &in, []string{"namespace", "key", "value_base64", "ttl_ms"})
	if e == nil {
		e = h.svc.SetCache(ctx, in.Namespace, in.Key, in.Value, in.TTL)
	}
	h.respond(c, "CACHE", struct{}{}, e)
}
func (h *Handler) DeleteCache(c *gin.Context) {
	ctx, ok := h.context(c, "CACHE", true)
	if !ok {
		return
	}
	e := emptyBody(c)
	if e == nil {
		e = h.svc.DeleteCache(ctx, c.Query("namespace"), c.Query("key"))
	}
	h.respond(c, "CACHE", struct{}{}, e)
}
func (h *Handler) CreateTask(c *gin.Context) {
	ctx, ok := h.context(c, "TASKCENTER", false)
	if !ok {
		return
	}
	var in svc.CreateTaskInput
	e := decode(c, &in, []string{"type", "idempotency_key", "payload"})
	var out *svc.Task
	if e == nil {
		out, e = h.svc.CreateTask(ctx, in)
	}
	h.respond(c, "TASKCENTER", out, e)
}
func (h *Handler) GetTask(c *gin.Context) {
	ctx, ok := h.context(c, "TASKCENTER", false)
	if !ok {
		return
	}
	if e := emptyBody(c); e != nil {
		h.respond(c, "TASKCENTER", nil, e)
		return
	}
	out, e := h.svc.GetTask(ctx, c.Param("task_uuid"))
	h.respond(c, "TASKCENTER", out, e)
}
func (h *Handler) UpdateTask(c *gin.Context) {
	ctx, ok := h.context(c, "TASKCENTER", false)
	if !ok {
		return
	}
	var in svc.UpdateTaskInput
	e := decode(c, &in, []string{"expected_revision", "state", "progress"}, "message_key", "result")
	var out *svc.Task
	if e == nil {
		out, e = h.svc.UpdateTask(ctx, c.Param("task_uuid"), in)
	}
	h.respond(c, "TASKCENTER", out, e)
}
