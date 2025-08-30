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

func HasAnyNonEmpty(m datatypes.JSONMap, keys ...string) bool {
	for _, k := range keys {
		if s := ToStr(m[k]); s != "" {
			return true
		}
	}
	return false
}
