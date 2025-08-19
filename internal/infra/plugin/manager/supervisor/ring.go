package supervisor

import (
	"bytes"
	"sync"
)

// ringBuffer 是固定容量的环形字节缓冲，Write 会覆盖最旧的数据。
// Snapshot(tailBytes) 返回复制后的最后 tailBytes（或更少）内容。
type ringBuffer struct {
	mu     sync.Mutex
	buf    []byte
	cap    int
	off    int  // 下次写入位置
	filled bool // 是否写满过
}

func newRingBuffer(capacity int) *ringBuffer {
	if capacity <= 0 {
		capacity = 256 * 1024
	}
	return &ringBuffer{
		buf: make([]byte, capacity),
		cap: capacity,
		off: 0,
	}
}

func (r *ringBuffer) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := len(p)
	if n >= r.cap {
		// 只保留最后 cap 字节
		copy(r.buf, p[n-r.cap:])
		r.off = 0
		r.filled = true
		return n, nil
	}
	// 分两段写（可能环绕）
	end := r.off + n
	if end <= r.cap {
		copy(r.buf[r.off:end], p)
		r.off = end % r.cap
	} else {
		part1 := r.cap - r.off
		copy(r.buf[r.off:], p[:part1])
		copy(r.buf[0:], p[part1:])
		r.off = (end % r.cap)
	}
	if r.off == 0 {
		r.filled = true
	}
	return n, nil
}

func (r *ringBuffer) Snapshot(tailBytes int) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	if tailBytes <= 0 || tailBytes > r.cap {
		tailBytes = r.cap
	}
	var out []byte
	if !r.filled {
		total := r.off
		if total == 0 {
			return nil
		}
		if tailBytes < total {
			out = make([]byte, tailBytes)
			copy(out, r.buf[total-tailBytes:total])
			return out
		}
		out = make([]byte, total)
		copy(out, r.buf[:total])
		return out
	}
	// filled：线性化为 [off,cap) + [0,off)
	tmp := make([]byte, r.cap)
	n := copy(tmp, r.buf[r.off:])
	n += copy(tmp[n:], r.buf[:r.off])
	if tailBytes < n {
		return bytes.Clone(tmp[n-tailBytes : n])
	}
	return bytes.Clone(tmp[:n])
}
