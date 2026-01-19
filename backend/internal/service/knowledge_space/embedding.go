package knowledge_space

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
)

func ContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func HashEmbedding(content string, dim int) []float32 {
	if dim <= 0 {
		dim = 32
	}
	sum := sha256.Sum256([]byte(content))
	vec := make([]float32, dim)
	for i := 0; i < dim; i++ {
		offset := (i * 4) % len(sum)
		u := binary.BigEndian.Uint32(sum[offset : offset+4])
		vec[i] = float32(u%10_000) / 10_000.0
	}
	return vec
}
