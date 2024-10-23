package organization

import (
	fmt "PowerX/pkg/printx"
	"PowerX/pkg/securityx"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_HashPassword(t *testing.T) {
	pwd := "root"
	encodedPassword := securityx.EncodePassword(pwd)
	fmt.Dump(encodedPassword)

	hashedPassword, _ := HashPassword(pwd)
	fmt.Dump(hashedPassword)

	//result := VerifyPassword("$2a$04$G68CjMtt1qCkD9.heW.d4ul7uER7SZIGP3gQJJzHgir4pJW1Mksre", pwd)
	result := VerifyPassword("$2a$04$j.nXwFJhAhr/oMebW42/H.EOFs8Hke8AZ2Lr3u/0b0vWZWuTVsEYC", encodedPassword)
	assert.EqualValues(t, true, result)
}
