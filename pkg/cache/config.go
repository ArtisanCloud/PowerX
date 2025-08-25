package cache

type CacheConfig struct {
	Driver       string `yaml:"driver" json:"driver"`
	Host         string `yaml:"host" json:"host"`
	Port         int    `yaml:"port" json:"port"`
	Password     string `yaml:"password" json:"password"`
	DB           int    `yaml:"db" json:"db"`
	PoolSize     int    `yaml:"pool_size" json:"pool_size"`
	MaxIdle      int    `yaml:"max_idle" json:"max_idle"`
	MaxActive    int    `yaml:"max_active" json:"max_active"`
	IdleTimeout  int    `yaml:"idle_timeout" json:"idle_timeout"`
	Timeout      int    `yaml:"timeout" json:"timeout"`
	ReadTimeout  int    `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout int    `yaml:"write_timeout" json:"write_timeout"`
	MaxRetries   int    `yaml:"max_retries" json:"max_retries"`
}
