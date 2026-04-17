package runtimebuffer

import (
	"bytes"
	"sync"

	"go.uber.org/zap/zapcore"
)

const defaultCapacity = 1024 * 1024

type ringBuffer struct {
	mu   sync.RWMutex
	buf  []byte
	cap  int
	head int
	size int
}

func newRing(capacity int) *ringBuffer {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	return &ringBuffer{buf: make([]byte, capacity), cap: capacity}
}

func (r *ringBuffer) append(p []byte) {
	if len(p) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, b := range p {
		r.buf[r.head] = b
		r.head = (r.head + 1) % r.cap
		if r.size < r.cap {
			r.size++
		}
	}
}

func (r *ringBuffer) snapshot(tailBytes int) []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.size == 0 {
		return nil
	}
	if tailBytes <= 0 || tailBytes > r.size {
		tailBytes = r.size
	}
	start := r.head - tailBytes
	if start < 0 {
		start += r.cap
	}
	out := make([]byte, tailBytes)
	if start+tailBytes <= r.cap {
		copy(out, r.buf[start:start+tailBytes])
		return out
	}
	first := r.cap - start
	copy(out, r.buf[start:])
	copy(out[first:], r.buf[:tailBytes-first])
	return out
}

var globalRing = newRing(defaultCapacity)

func Append(p []byte) {
	globalRing.append(p)
}

func Snapshot(tailBytes int) []byte {
	return bytes.Clone(globalRing.snapshot(tailBytes))
}

type teeWriteSyncer struct {
	primary zapcore.WriteSyncer
}

func NewTeeWriteSyncer(primary zapcore.WriteSyncer) zapcore.WriteSyncer {
	if primary == nil {
		primary = zapcore.AddSync(bytes.NewBuffer(nil))
	}
	return &teeWriteSyncer{primary: primary}
}

func (t *teeWriteSyncer) Write(p []byte) (int, error) {
	if len(p) > 0 {
		Append(p)
	}
	return t.primary.Write(p)
}

func (t *teeWriteSyncer) Sync() error {
	return t.primary.Sync()
}
