package securityx

import (
	"crypto/md5"
	"fmt"
)

func HashStringData(strData string) string {
	data := []byte(strData)
	hashKey := md5.Sum(data)

	return fmt.Sprintf("%x", hashKey)
}
