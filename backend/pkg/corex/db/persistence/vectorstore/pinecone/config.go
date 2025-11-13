package pinecone

import "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"

// Config 占位配置结构。
type Config struct {
	Endpoint  string `yaml:"endpoint"`
	APIKey    string `yaml:"api_key"`
	Index     string `yaml:"index"`
	Namespace string `yaml:"namespace"`
}

func init() {
	vectorstore.Register(vectorstore.DriverPinecone, func(options interface{}) (vectorstore.Store, error) {
		return nil, vectorstore.ErrNotImplemented
	})
}
