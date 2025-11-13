package milvus

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"

// Config 保留占位结构，便于后续实现。
type Config struct {
	Endpoint string `yaml:"endpoint"`
	APIKey   string `yaml:"api_key"`
	Project  string `yaml:"project"`
}

func init() {
	vectorstore.Register(vectorstore.DriverMilvus, func(options interface{}) (vectorstore.Store, error) {
		return nil, vectorstore.ErrNotImplemented
	})
}
