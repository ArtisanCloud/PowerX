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

func MaskName(name string) (maskedName string) {
	nameLen := len(name)
	if nameLen > 2 {
		maskedName = name[:2] + strings.Repeat("*", nameLen-2)
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
