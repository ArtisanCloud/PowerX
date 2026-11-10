package writer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	logcfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
)

// LokiDeliveryStats 是普通运行日志的进程级交付计量，不代表可靠审计已持久化。
type LokiDeliveryStats struct {
	Accepted      uint64 `json:"accepted"`
	Delivered     uint64 `json:"delivered"`
	Dropped       uint64 `json:"dropped"`
	Failed        uint64 `json:"failed"`
	Retries       uint64 `json:"retries"`
	QueuedEntries int64  `json:"queued_entries"`
	QueuedBytes   int64  `json:"queued_bytes"`
	QueueCapacity int    `json:"queue_capacity"`
	MaxQueueBytes int64  `json:"max_queue_bytes"`
	LastError     string `json:"last_error,omitempty"`
	Closed        bool   `json:"closed"`
}
type lokiSettings struct {
	capacity, batchSize, entryBytes         int
	queueBytes                              int64
	batchWait, requestTimeout, flushTimeout time.Duration
}

func deliverySettings(c *logcfg.LokiConfig) lokiSettings {
	s := lokiSettings{capacity: 512, batchSize: 100, entryBytes: 64 << 10, queueBytes: 8 << 20, batchWait: time.Second, requestTimeout: 2 * time.Second, flushTimeout: 2 * time.Second}
	if c == nil {
		return s
	}
	if c.QueueCapacity > 0 && c.QueueCapacity <= 10000 {
		s.capacity = c.QueueCapacity
	}
	if c.BatchSize > 0 && c.BatchSize <= 1000 {
		s.batchSize = c.BatchSize
	}
	if c.MaxEntryBytes > 0 && c.MaxEntryBytes <= 1<<20 {
		s.entryBytes = c.MaxEntryBytes
	}
	if c.MaxQueueBytes > 0 && c.MaxQueueBytes <= 64<<20 {
		s.queueBytes = c.MaxQueueBytes
	}
	if c.BatchWait > 0 && c.BatchWait <= 10 {
		s.batchWait = time.Duration(c.BatchWait) * time.Second
	}
	if c.RequestTimeoutMS > 0 && c.RequestTimeoutMS <= 10000 {
		s.requestTimeout = time.Duration(c.RequestTimeoutMS) * time.Millisecond
	}
	if c.FlushTimeoutMS > 0 && c.FlushTimeoutMS <= 10000 {
		s.flushTimeout = time.Duration(c.FlushTimeoutMS) * time.Millisecond
	}
	return s
}

type lokiEntry struct {
	timestamp string
	body      []byte
}
type lokiDelivery struct {
	w                                                        *LokiWriter
	settings                                                 lokiSettings
	queue                                                    chan lokiEntry
	flush, progress                                          chan struct{}
	done                                                     chan struct{}
	ctx                                                      context.Context
	cancel                                                   context.CancelFunc
	start                                                    sync.Once
	mu                                                       sync.RWMutex
	closed                                                   bool
	accepted, delivered, dropped, failed, retries, completed atomic.Uint64
	queuedBytes, queuedEntries                               atomic.Int64
	lastError                                                atomic.Value
}

func newLokiDelivery(w *LokiWriter, s lokiSettings) *lokiDelivery {
	ctx, cancel := context.WithCancel(context.Background())
	d := &lokiDelivery{w: w, settings: s, queue: make(chan lokiEntry, s.capacity), flush: make(chan struct{}, 1), progress: make(chan struct{}, 1), done: make(chan struct{}), ctx: ctx, cancel: cancel}
	d.lastError.Store("")
	return d
}
func (d *lokiDelivery) write(p []byte) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return 0, errors.New("Loki delivery closed")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if len(p) > d.settings.entryBytes {
		d.dropped.Add(1)
		d.lastError.Store("entry_too_large")
		return len(p), nil
	}
	for {
		n := d.queuedBytes.Load()
		if n+int64(len(p)) > d.settings.queueBytes {
			d.dropped.Add(1)
			d.lastError.Store("queue_full")
			return len(p), nil
		}
		if d.queuedBytes.CompareAndSwap(n, n+int64(len(p))) {
			break
		}
	}
	entry := lokiEntry{timestamp: strconv.FormatInt(time.Now().UnixNano(), 10), body: append([]byte(nil), p...)}
	d.queuedEntries.Add(1)
	select {
	case d.queue <- entry:
		d.accepted.Add(1)
		d.start.Do(func() { go d.run() })
	default:
		d.queuedBytes.Add(-int64(len(p)))
		d.queuedEntries.Add(-1)
		d.dropped.Add(1)
		d.lastError.Store("queue_full")
	}
	// 不向 zap 返回队列错误，避免错误日志再次写入同一个失败出口形成循环。
	return len(p), nil
}
func (d *lokiDelivery) finish(batch []lokiEntry, err error) {
	if err == nil {
		d.delivered.Add(uint64(len(batch)))
		d.lastError.Store("")
	} else {
		d.failed.Add(uint64(len(batch)))
		d.lastError.Store(err.Error())
	}
	var n int64
	for _, entry := range batch {
		n += int64(len(entry.body))
	}
	d.queuedBytes.Add(-n)
	d.queuedEntries.Add(-int64(len(batch)))
	d.completed.Add(uint64(len(batch)))
	select {
	case d.progress <- struct{}{}:
	default:
	}
}
func (d *lokiDelivery) run() {
	defer close(d.done)
	defer d.cancel()
	tick := time.NewTicker(d.settings.batchWait)
	defer tick.Stop()
	batch := make([]lokiEntry, 0, d.settings.batchSize)
	send := func() {
		if len(batch) > 0 {
			d.finish(batch, d.send(batch))
			batch = batch[:0]
		}
	}
	for {
		select {
		case e, ok := <-d.queue:
			if !ok {
				send()
				return
			}
			batch = append(batch, e)
			if len(batch) >= d.settings.batchSize {
				send()
			}
		case <-tick.C:
			send()
		case <-d.flush:
			draining := true
			for draining {
				select {
				case e, ok := <-d.queue:
					if !ok {
						send()
						return
					}
					batch = append(batch, e)
					if len(batch) >= d.settings.batchSize {
						send()
					}
				default:
					draining = false
				}
			}
			send()
		case <-d.ctx.Done():
			if len(batch) > 0 {
				d.finish(batch, errors.New("shutdown_timeout"))
			}
			for {
				select {
				case e, ok := <-d.queue:
					if !ok {
						return
					}
					d.finish([]lokiEntry{e}, errors.New("shutdown_timeout"))
				default:
					return
				}
			}
		}
	}
}
func (d *lokiDelivery) send(batch []lokiEntry) error {
	values := make([][]string, 0, len(batch))
	for _, e := range batch {
		values = append(values, []string{e.timestamp, string(e.body)})
	}
	body, err := json.Marshal(map[string]any{"streams": []any{map[string]any{"stream": d.w.labels, "values": values}}})
	if err != nil {
		return errors.New("encoding_failed")
	}
	last := errors.New("transport_error")
	for attempt := 0; attempt < d.w.retryCount; attempt++ {
		if d.ctx.Err() != nil {
			return errors.New("shutdown_timeout")
		}
		if attempt > 0 {
			d.retries.Add(1)
			timer := time.NewTimer(time.Duration(attempt) * 100 * time.Millisecond)
			select {
			case <-timer.C:
			case <-d.ctx.Done():
				timer.Stop()
				return errors.New("shutdown_timeout")
			}
		}
		req, e := http.NewRequestWithContext(d.ctx, http.MethodPost, d.w.url, bytes.NewReader(body))
		if e != nil {
			return errors.New("invalid_push_url")
		}
		req.Header.Set("Content-Type", "application/json")
		resp, e := d.w.client.Do(req)
		if e != nil {
			last = errors.New("transport_error")
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusNoContent {
			return nil
		}
		last = errors.New("http_" + strconv.Itoa(resp.StatusCode))
		if resp.StatusCode < 500 && resp.StatusCode != 429 {
			return last
		}
	}
	return last
}
func (d *lokiDelivery) sync() error {
	target := d.accepted.Load()
	select {
	case d.flush <- struct{}{}:
	default:
	}
	timer := time.NewTimer(d.settings.flushTimeout)
	defer timer.Stop()
	for d.completed.Load() < target {
		select {
		case <-d.progress:
		case <-d.done:
		case <-timer.C:
			if d.completed.Load() < target {
				return errors.New("Loki flush timeout")
			}
		}
	}
	return nil
}
func (d *lokiDelivery) close() error {
	d.mu.Lock()
	if !d.closed {
		d.closed = true
		close(d.queue)
		d.start.Do(func() { go d.run() })
	}
	d.mu.Unlock()
	timer := time.NewTimer(d.settings.flushTimeout)
	defer timer.Stop()
	select {
	case <-d.done:
		return nil
	case <-timer.C:
		d.cancel()
		<-d.done
		return errors.New("Loki shutdown flush timeout")
	}
}
func (d *lokiDelivery) stats() LokiDeliveryStats {
	d.mu.RLock()
	closed := d.closed
	d.mu.RUnlock()
	return LokiDeliveryStats{Accepted: d.accepted.Load(), Delivered: d.delivered.Load(), Dropped: d.dropped.Load(), Failed: d.failed.Load(), Retries: d.retries.Load(), QueuedEntries: d.queuedEntries.Load(), QueuedBytes: d.queuedBytes.Load(), QueueCapacity: d.settings.capacity, MaxQueueBytes: d.settings.queueBytes, LastError: d.lastError.Load().(string), Closed: closed}
}
