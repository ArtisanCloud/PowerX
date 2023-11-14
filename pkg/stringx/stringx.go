package stringx

import (
	"math/rand"
	"time"
)

func GenerateRandomCode(num int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())

	code := make([]byte, num)
	for i := 0; i < num; i++ {
		code[i] = charset[rand.Intn(len(charset))]
	}

	return string(code)
}
