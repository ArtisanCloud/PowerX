package origanzation

import (
	fmt "PowerX/pkg/printx"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_HashPassword(t *testing.T) {
	pwd := "root"
	encodedPassword, _ := HashPassword(pwd)
	fmt.Dump(encodedPassword)

	result := VerifyPassword("$2a$04$G68CjMtt1qCkD9.heW.d4ul7uER7SZIGP3gQJJzHgir4pJW1Mksre", pwd)
	assert.EqualValues(t, true, result)
}
