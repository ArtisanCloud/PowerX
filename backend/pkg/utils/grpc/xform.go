package grpc

import "google.golang.org/protobuf/types/known/structpb"

/******** utils：最小化 struct 构造与取值 ********/
func MustStruct(m map[string]any) *structpb.Struct {
	if m == nil {
		s, _ := structpb.NewStruct(map[string]any{})
		return s
	}
	s, _ := structpb.NewStruct(m)
	return s
}
func MustStructFromAny(v any) *structpb.Struct {
	switch x := v.(type) {
	case nil:
		return MustStruct(nil)
	case *structpb.Struct:
		return x
	case map[string]any:
		return MustStruct(x)
	default:
		// 统一包一层 value 兜底
		return MustStruct(map[string]any{"value": x})
	}
}

func MergeStruct(dst map[string]any, st *structpb.Struct) {
	if st == nil {
		return
	}
	for k, v := range st.AsMap() {
		dst[k] = v
	}
}
