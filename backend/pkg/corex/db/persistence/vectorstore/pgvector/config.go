package pgvector

// Config 描述 pgvector 驱动的必需参数。
type Config struct {
	DSN              string `yaml:"dsn"`
	Schema           string `yaml:"schema"`
	Table            string `yaml:"table"`
	Dimensions       int    `yaml:"dimensions"`
	EnableMigrations bool   `yaml:"enable_migrations"`
	BatchSize        int    `yaml:"batch_size"`
	Lists            int    `yaml:"ivfflat_lists"`
	TimeoutSeconds   int    `yaml:"timeout_seconds"`
}

// WithDefaults 填充缺省值。
func (c Config) WithDefaults() Config {
	if c.Schema == "" {
		c.Schema = "public"
	}
	if c.Table == "" {
		c.Table = "knowledge_vectors_v1_1536"
	}
	if c.Dimensions <= 0 {
		c.Dimensions = 1536
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 128
	}
	if c.Lists <= 0 {
		c.Lists = 100
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = 30
	}
	return c
}
