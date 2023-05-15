package securityx

import (
	"crypto/md5"
	"fmt"
	"strings"
)

func HashStringData(strData string) string {
	data := []byte(strData)
	hashKey := md5.Sum(data)

	return fmt.Sprintf("%x", hashKey)
}

func MaskName(name string, maskLen int) (maskedName string) {
	runes := []rune(name)
	nameLen := len(runes)
	if nameLen > 2 {
		// 是否要定制脱敏长度
		if maskLen >= 0 && maskLen < nameLen {
			nameLen = maskLen
		}
		maskedName = string(runes[:2]) + strings.Repeat("*", nameLen-2)
	} else {
		maskedName = strings.Repeat("*", nameLen)
	}

	return maskedName
}

func MaskMobile(mobile string) (maskedMobile string) {
	mobileLen := len(mobile)
	if mobileLen > 7 {
		maskedMobile = mobile[:3] + strings.Repeat("*", mobileLen-7) + mobile[mobileLen-4:]
	} else {
		maskedMobile = strings.Repeat("*", mobileLen)
	}
	return maskedMobile
}
