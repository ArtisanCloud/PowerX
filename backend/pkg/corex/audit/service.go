package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"time"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/audit"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

// —— 只认 GORM 模型 ——

// Repository：直接用你 db 层的仓储（签名就是 []dbm.AuditEvent）
type Repository interface {
	InsertBatch(ctx context.Context, rows []dbm.AuditEvent) error
}

// Sink：旁路（日志、总线…）
type Sink interface {
	Emit(ctx context.Context, evt *dbm.AuditEvent) error
}

type AuditOptions struct {
	BatchSize      int
	BatchWait      time.Duration
	MaxPayloadSize int
	EnableHash     bool
	HashSecret     []byte
}

type Service interface {
	Emit(ctx context.Context, evt *dbm.AuditEvent) error
	Close()
}

// ServiceOptions 汇集 Service 构造依赖。
type ServiceOptions struct {
	DB         *gorm.DB
	Repository Repository
	Sinks      []Sink
	Config     AuditOptions
}

type serviceImpl struct {
	repo  Repository
	sinks []Sink
	opt   AuditOptions

	ch    chan *dbm.AuditEvent
	stopC chan struct{}

	consecFail int
	dropUntil  time.Time
}

func NewService(opts ServiceOptions) Service {
	repoImpl := opts.Repository
	if repoImpl == nil {
		if opts.DB == nil {
			panic("audit.Service requires DB when Repository is nil")
		}
		repoImpl = repo.NewAuditEventRepository(opts.DB)
	}
	sinks := opts.Sinks
	if sinks == nil {
		sinks = []Sink{}
	}
	opt := opts.Config
	if opt.BatchSize <= 0 {
		opt.BatchSize = 100
	}
	if opt.BatchWait <= 0 {
		opt.BatchWait = 200 * time.Millisecond
	}
	s := &serviceImpl{
		repo:  repoImpl,
		sinks: sinks,
		opt:   opt,
		ch:    make(chan *dbm.AuditEvent, opt.BatchSize*8),
		stopC: make(chan struct{}),
	}
	go s.loop()
	return s
}

func (s *serviceImpl) Emit(ctx context.Context, evt *dbm.AuditEvent) error {
	select {
	case s.ch <- evt:
		return nil
	default:
		// 背压降级：Meta 只留摘要
		evt.Meta = mustJSON(map[string]any{"_dropped_detail": true})
		s.ch <- evt
		return nil
	}
}

func (s *serviceImpl) Close() { close(s.stopC) }

func (s *serviceImpl) loop() {
	t := time.NewTimer(s.opt.BatchWait)
	defer t.Stop()
	var buf []*dbm.AuditEvent

	flush := func() {
		if len(buf) == 0 {
			return
		}
		// 预处理（截断/哈希）
		rows := make([]dbm.AuditEvent, 0, len(buf))
		for _, e := range buf {
			s.prepare(e)
			rows = append(rows, *e)
		}
		now := time.Now()
		if !now.Before(s.dropUntil) {
			if err := s.repo.InsertBatch(context.Background(), rows); err != nil {
				pxlog.Error(pxlog.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "audit insert batch failed: "+err.Error())
				s.consecFail++
				backoffs := []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 5 * time.Minute}
				idx := s.consecFail - 1
				if idx >= len(backoffs) {
					idx = len(backoffs) - 1
				}
				s.dropUntil = now.Add(backoffs[idx])
			} else {
				s.consecFail = 0
				s.dropUntil = time.Time{}
			}
		} else {
			// 熔断窗口内：跳过落库，只旁路
		}

		for _, e := range buf {
			for _, sk := range s.sinks {
				_ = sk.Emit(context.Background(), e)
			}
		}
		buf = buf[:0]
	}

	for {
		select {
		case <-s.stopC:
			flush()
			return
		case e := <-s.ch:
			buf = append(buf, e)
			if len(buf) >= s.opt.BatchSize {
				flush()
				if !t.Stop() {
					<-t.C
				}
				t.Reset(s.opt.BatchWait)
			}
		case <-t.C:
			flush()
			t.Reset(s.opt.BatchWait)
		}
	}
}

func (s *serviceImpl) prepare(e *dbm.AuditEvent) {
	// OccurredAt
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now()
	}

	if e.ClientIP != nil && *e.ClientIP == "" {
		e.ClientIP = nil
	}

	// 截断 Meta
	if s.opt.MaxPayloadSize > 0 && len(e.Meta) > s.opt.MaxPayloadSize {
		e.Meta = mustJSON(map[string]any{"_truncated": true, "size": len(e.Meta)})
	}

	// （可选）哈希
	if s.opt.EnableHash && len(s.opt.HashSecret) > 0 {
		h := hmac.New(sha256.New, s.opt.HashSecret)
		write := func(s string) { h.Write([]byte(s)); h.Write([]byte{0}) }
		write(e.Source)
		write(e.Operation)
		write(e.ResourceType)
		write(e.ResourceID)
		write(e.Outcome)
		write(e.Severity)
		e.Hash = hex(h.Sum(nil))
	}
}

func hex(b []byte) string {
	const d = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = d[v>>4]
		out[i*2+1] = d[v&0x0f]
	}
	return string(out)
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
