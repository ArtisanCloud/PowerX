package utils

import (
	"encoding/json"
	"gorm.io/datatypes"
)

func J(v any) datatypes.JSON {
	b, _ := json.Marshal(v)
	return datatypes.JSON(b)
}

func MustJSONBytes(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
