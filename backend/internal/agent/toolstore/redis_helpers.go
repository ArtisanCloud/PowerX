package toolstore

import "reflect"

func isNilUniversalClient(client interface{}) bool {
	if client == nil {
		return true
	}
	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Slice, reflect.Map, reflect.Func:
		return value.IsNil()
	default:
		return false
	}
}
