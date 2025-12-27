package capability_registry

import (
	"reflect"

	"github.com/redis/go-redis/v9"
)

func isNilRedisClientValue(client interface{}) bool {
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

func isNilUniversalClient(client redis.UniversalClient) bool {
	if client == nil {
		return true
	}
	return isNilRedisClientValue(client)
}
