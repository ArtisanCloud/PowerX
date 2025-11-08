// pkg/utils/random.go
package utils

import (
	"crypto/rand"
	"math/big"
)

const defaultAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandomString 返回由 [a-zA-Z0-9] 组成的长度为 n 的随机字符串（加密安全）。
// 失败时会 panic（通常只有极端系统熵不足才会发生）。
func RandomString(n int) string {
	return RandomStringWithAlphabet(n, defaultAlphabet)
}

// RandomStringWithAlphabet 使用自定义字母表生成随机字符串（加密安全）。
// 若 alphabet 为空，则回退到默认字母表。
func RandomStringWithAlphabet(n int, alphabet string) string {
	if n <= 0 {
		return ""
	}
	if alphabet == "" {
		alphabet = defaultAlphabet
	}
	buf := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := 0; i < n; i++ {
		r, err := rand.Int(rand.Reader, max) // 无偏采样
		if err != nil {
			panic(err) // 保持“Must”语义，避免静默降级为不安全随机
		}
		buf[i] = alphabet[r.Int64()]
	}
	return string(buf)
}

// RandomBytes 生成 n 字节的加密安全随机字节序列。
// 失败时会 panic。
func RandomBytes(n int) []byte {
	if n <= 0 {
		return []byte{}
	}
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
