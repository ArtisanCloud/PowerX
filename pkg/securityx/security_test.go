package securityx

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_EncodePassword(t *testing.T) {
	encodedPassword := EncodePassword("111111")

	assert.EqualValues(t, "bcb15f821479b4d5772bd0ca866c00ad5f926e3580720659cc80d39c9d09802a", encodedPassword)
}

func Test_HashPlainPassword(t *testing.T) {

	encodePassword := EncodePassword("111111")
	hashedPassword := HashPlainPassword("111111")
	result := CheckPassword(hashedPassword, encodePassword)
	//result := CheckPassword(hashPassword, "7bcb20c97083e9f160d02491e2c51af06ffc359d708235169a0db2fdeb575a1e")
	assert.EqualValues(t, true, result)

}
