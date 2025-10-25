package adapter

import (
	"github.com/ArtisanCloud/PowerX/pkg/dynamic_form/dto"
	"github.com/ArtisanCloud/PowerX/pkg/dynamic_form/model"
)

// MapToToolInput 将 validated form inputs 按映射表转换成目标工具/Node 参数
// mapping: targetKey -> sourceFieldName
func MapToToolInput(validated map[string]interface{}, mapping map[string]string) map[string]interface{} {
	output := make(map[string]interface{})
	for target, source := range mapping {
		if v, ok := validated[source]; ok {
			output[target] = v
		}
	}
	return output
}

func RequestToDomainForm(req *dto.CreateFormRequest) *model.FormSchema {
	return req.FormSchema
}
