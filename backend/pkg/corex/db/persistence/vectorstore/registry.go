package vectorstore

import (
	"fmt"
	"strings"
	"sync"
)

// Factory 表示驱动构造函数，options 的具体类型由驱动自行定义。
type Factory func(options interface{}) (Store, error)

var (
	registry sync.Map
)

// Register 将驱动注册到全局表。
func Register(name string, factory Factory) {
	if strings.TrimSpace(name) == "" || factory == nil {
		return
	}
	key := strings.ToLower(strings.TrimSpace(name))
	registry.Store(key, factory)
}

// Open 根据驱动名称实例化 Store。
func Open(name string, options interface{}) (Store, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return nil, fmt.Errorf("%w: empty driver", ErrUnknownDriver)
	}
	factoryValue, ok := registry.Load(key)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownDriver, key)
	}
	factory := factoryValue.(Factory)
	store, err := factory(options)
	if err != nil {
		return nil, err
	}
	if store == nil {
		return nil, fmt.Errorf("vectorstore: factory %s returned nil store", key)
	}
	return store, nil
}
