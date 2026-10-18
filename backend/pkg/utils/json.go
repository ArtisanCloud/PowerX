package utils

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"gorm.io/datatypes"
	"io"
	"unicode/utf8"
)

//go:embed json.go
var strictJSONImplementation []byte

// StrictJSONImplementationBytes 为内置 typed 工具发布包提供实际编译源原件。
func StrictJSONImplementationBytes() []byte { return append([]byte(nil), strictJSONImplementation...) }

// DecodeStrictJSON 拒绝重复字段、未知字段、非法 UTF-8、过深嵌套与尾随 JSON。
// 大小限制由所属协议在调用前执行，避免通用工具决定业务上限。
func DecodeStrictJSON(raw []byte, target any) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("json.invalid_utf8")
	}
	tokens := json.NewDecoder(bytes.NewReader(raw))
	tokens.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 128 {
			return fmt.Errorf("json.depth_limit")
		}
		token, err := tokens.Token()
		if err != nil {
			return err
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for tokens.More() {
				key, err := tokens.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return fmt.Errorf("json.duplicate_or_invalid_field")
				}
				seen[name] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = tokens.Token()
		case json.Delim('['):
			for tokens.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			_, err = tokens.Token()
		}
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return fmt.Errorf("json.trailing_content")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

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

func CloneJSONMap(m datatypes.JSONMap) datatypes.JSONMap {
	if m == nil {
		return datatypes.JSONMap{}
	}
	clone := datatypes.JSONMap{}
	for k, v := range m {
		clone[k] = v
	}
	return clone
}
