package core

import "bytes"

// TrimDataPrefix trims a "data:" prefix for SSE-style lines.
func TrimDataPrefix(line []byte) []byte {
	b := bytes.TrimSpace(line)
	if bytes.HasPrefix(b, []byte("data:")) {
		b = b[len("data:"):]
		b = bytes.TrimLeft(b, " \t")
	}
	return b
}
