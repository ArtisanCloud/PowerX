package product

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSKU_GetComposedUniqueID(t *testing.T) {
	sku := SKU{
		ProductId: 40,
		// 注意这里的json不能有空格
		OptionIds: []byte(`[235,240]`),
	}

	expectedUniqueID := "42f065a6a7bdf58e09ad25e9cf5b2031" // 假设使用MD5进行哈希计算

	composedUniqueID := sku.GetComposedUniqueID()
	assert.True(t, composedUniqueID.Valid)
	assert.Equal(t, expectedUniqueID, composedUniqueID.String)
}

func TestSKU_GetComposedUniqueID_Invalid(t *testing.T) {
	sku := SKU{
		ProductId: 123,
		OptionIds: []byte{},
	}

	composedUniqueID := sku.GetComposedUniqueID()
	assert.False(t, composedUniqueID.Valid)
	assert.Empty(t, composedUniqueID.String)
}
