package db

// 数据库配置
type DatabaseConfig struct {
	Driver                 string          `yaml:"driver"`                    // 数据库驱动（mysql/postgres等）
	DSN                    string          `yaml:"dsn"`                       // 数据源名称（DSN）
	Host                   string          `yaml:"host"`                      // 数据库主机地址
	Port                   int             `yaml:"port"`                      // 数据库端口
	UserName               string          `yaml:"username"`                  // 数据库用户名
	Password               string          `yaml:"password"`                  // 数据库密码
	Database               string          `yaml:"database"`                  // 数据库名称
	SSLMode                string          `yaml:"ssl_mode"`                  // SSL模式（disable/require等）
	Timezone               string          `yaml:"timezone"`                  // 时区设置
	TablePrefix            string          `yaml:"table_prefix"`              // 表前缀
	MaxIdleConns           int             `yaml:"max_idle_conns"`            // 最大空闲连接数
	MaxOpenConns           int             `yaml:"max_open_conns"`            // 最大打开连接数
	ConnMaxLifetimeMinutes int             `yaml:"conn_max_lifetime_minutes"` // 连接最大生命周期（分钟）
	LogLevel               string          `yaml:"log_level"`                 // 日志级别（silent/error/warn/info）
	ClusterMode            string          `yaml:"cluster_mode"`              // single / primary-replica / sharded
	Replicas               []ReplicaConfig `yaml:"replicas"`                  // 只读或备用节点列表
}

// ReplicaConfig 描述从库/只读节点
type ReplicaConfig struct {
	Role     string `yaml:"role"` // read / standby / custom
	DSN      string `yaml:"dsn"`  // 可用单独 DSN
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}
