package powermodel

import (
	fmt "PowerX/pkg/printx"
	"reflect"
	"testing"
)

func TestGetModelFields(t *testing.T) {
	type SpecificOption struct {
		ProductSpecificId int64  `gorm:"comment: 产品规格Id;index;not null" json:"productSpecificId"`
		Name              string `gorm:"comment: 规格项名称;not null" json:"name"`
		IsActivated       bool   `gorm:"comment: 是否被激活;" json:"isActivated"`
	}

	expectedFields := []string{"product_specific_id", "name", "is_activated"}

	model := SpecificOption{}
	fields := GetModelFields(model)
	fmt.Dump(fields)

	if !reflect.DeepEqual(fields, expectedFields) {
		t.Errorf("Expected fields %v, but got %v", expectedFields, fields)
	}
}
