package pluginx

import "fmt"

type Config interface {
	Default() Config
	Validate() error
	As(c Config) error
}

// BuildLoaderConfig 插件构建加载器配置
type BuildLoaderConfig struct {
	MainAPIEndpoint string
}

func (b *BuildLoaderConfig) Default() Config {
	if b.MainAPIEndpoint == "" {
		b.MainAPIEndpoint = "/api/plugin"
	}
	return b
}

func (b *BuildLoaderConfig) Validate() error {
	return nil
}

func (b *BuildLoaderConfig) As(c Config) error {
	if bc, ok := c.(*BuildLoaderConfig); ok {
		bc.MainAPIEndpoint = b.MainAPIEndpoint
		return nil
	}
	return fmt.Errorf("config type not match")
}
