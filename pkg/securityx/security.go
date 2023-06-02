package securityx

import (
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"log"
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

func EncodePassword(plainPassword string) (strEncode string) {

	encoded := sha256.Sum256([]byte(plainPassword))
	strEncode = fmt.Sprintf("%x", encoded)

	//fmt.Println("encoded password", strEncode)

	return strEncode

}

func HashPassword(encodedPassword string) (hashedPassword string) {

	hashed, err := bcrypt.GenerateFromPassword([]byte(encodedPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	//fmt.Println("Hashed password", string(encoded))

	return string(hashed)
}

func HashPlainPassword(plainPassword string) (hashedPassword string) {
	encodedPassword := EncodePassword(plainPassword)
	hashedPassword = HashPassword(encodedPassword)

	return hashedPassword
}

/*
CompareHashAndPassword 参数说明：

	hashedPassword：表示已经通过哈希算法处理过的密码哈希值（通常是使用bcrypt或其他密码哈希算法生成的）。
	password：表示用户提供的明文密码。
	函数返回一个error类型的值，其含义如下：

	如果密码哈希值和明文密码匹配，则返回nil，表示匹配成功。
	如果密码哈希值和明文密码不匹配，则返回bcrypt.ErrMismatchedHashAndPassword错误，表示匹配失败。
	CompareHashAndPassword函数内部会根据提供的哈希算法和比较规则，将明文密码进行哈希处理，并与已有的密码哈希值进行比较。如果二者匹配，则表示密码验证成功。

	这个函数在进行密码验证时非常有用，可以帮助开发者确保用户输入的密码与存储的密码哈希值匹配，从而提高系统的安全性。
*/
func CheckPassword(hashedPassword string, encodedPassword string) (isPasswordValid bool) {

	fmt.Printf("hashedPassword %s\r\n", hashedPassword)
	fmt.Printf("encodedPassword %s\n", encodedPassword)

	if err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(encodedPassword)); err != nil {
		fmt.Printf("%x", err)
		return false
	}

	return true
}
