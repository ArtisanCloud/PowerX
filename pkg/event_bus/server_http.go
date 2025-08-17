package event_bus

import (
	"encoding/json"
	"net/http"
	"time"
)

type Envelope struct {
	Topic string                 `json:"topic"`
	Data  map[string]interface{} `json:"data"`
}

type ServerOptions struct {
	Backend     EventBus
	Router      Router
	Auditor     Auditor
	HTTPClient  *http.Client
	Timeout     time.Duration
	MaxAttempts int
	BaseBackoff time.Duration
}

type httpServer struct {
	backend     EventBus
	router      Router
	auditor     Auditor
	httpClient  *http.Client
	timeout     time.Duration
	maxAttempts int
	baseBackoff time.Duration
}

func RegisterAPIRoutes(mux *http.ServeMux, opts ServerOptions) {
	if opts.Auditor == nil {
		opts.Auditor = NoopAuditor{}
	}
	if opts.Timeout == 0 {
		opts.Timeout = 2 * time.Second
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 3
	}
	if opts.BaseBackoff == 0 {
		opts.BaseBackoff = 200 * time.Millisecond
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 2 * time.Second}
	}

	s := &httpServer{backend: opts.Backend, router: opts.Router, auditor: opts.Auditor,
		httpClient: opts.HTTPClient, timeout: opts.Timeout, maxAttempts: opts.MaxAttempts, baseBackoff: opts.BaseBackoff}

	mux.HandleFunc("/_bus/publish", s.handlePublish)
}

func (s *httpServer) handlePublish(w http.ResponseWriter, r *http.Request) {
	var env Envelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil || env.Topic == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	// TODO: 这里可接你现有 auth，读取 tenant/trace 放进 ctx

	// 可选：写入底层总线，供进程内消费者使用（你已有 Publish 实现）:contentReference[oaicite:12]{index=12}
	_ = s.backend.Publish // 若要无返回值，可忽略错误并直接调用
	s.backend.Publish(env.Topic, env.Data, r.Context())

	subs := s.router.Subscribers(env.Topic)
	s.auditor.LogPublish(r.Context(), env.Topic, len(subs))

	for _, sub := range subs {
		go s.deliver(r.Context(), sub, env) // 异步投递到各插件 /events
	}
	w.WriteHeader(http.StatusAccepted)
}
