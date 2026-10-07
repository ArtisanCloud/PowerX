package evidence

import (
	"reflect"
	"strings"
	"time"
)

// ReportJSONSchema exposes the same typed contract used by the runtime validator.
func ReportJSONSchema() map[string]any {
	out := typeSchema(reflect.TypeOf(Report{}))
	out["$id"] = ReportSchema
	out["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	props := out["properties"].(map[string]any)
	props["schema"] = map[string]any{"const": ReportSchema}
	props["outcome"] = map[string]any{"enum": []string{"completed", "needs_action", "blocked", "failed"}}
	return out
}

func typeSchema(t reflect.Type) map[string]any {
	if t == reflect.TypeOf(time.Time{}) {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int:
		return map[string]any{"type": "integer"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": typeSchema(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": typeSchema(t.Elem())}
	case reflect.Struct:
		properties := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			key := strings.Split(f.Tag.Get("json"), ",")[0]
			if key == "" || key == "-" {
				continue
			}
			properties[key] = typeSchema(f.Type)
			required = append(required, key)
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	default:
		panic("evidence.schema_type_unsupported")
	}
}
