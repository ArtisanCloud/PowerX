package backup_ops

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 使用正在运行的数据库连接配置，避免配置文件和环境覆盖后指向不同库。
func sourceDSN(db *gorm.DB) string {
	if db != nil {
		if dialector, ok := db.Dialector.(*postgres.Dialector); ok && dialector.Config != nil {
			return dialector.Config.DSN
		}
		return ""
	}
	return ""
}

var libpqOption = regexp.MustCompile(`(?:^|\s)(sslmode|sslrootcert|sslcert|sslkey)=(?:'([^']*)'|([^\s]+))`)

// 密码只放入子进程环境，不进入命令行、响应或任务日志。
func postgresEnvironment(dsn string) ([]string, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("backup source database is not configured")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid backup source connection")
	}
	opts := map[string]string{}
	if u, e := url.Parse(dsn); e == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		for _, k := range []string{"sslmode", "sslrootcert", "sslcert", "sslkey"} {
			opts[k] = u.Query().Get(k)
		}
	} else {
		for _, m := range libpqOption.FindAllStringSubmatch(dsn, -1) {
			v := m[2]
			if v == "" {
				v = m[3]
			}
			opts[m[1]] = v
		}
	}
	if opts["sslmode"] == "" {
		opts["sslmode"] = "prefer"
		if cfg.TLSConfig == nil {
			opts["sslmode"] = "disable"
		}
	}
	env := []string{"PGHOST=" + cfg.Host, "PGPORT=" + strconv.Itoa(int(cfg.Port)), "PGUSER=" + cfg.User, "PGPASSWORD=" + cfg.Password, "PGDATABASE=" + cfg.Database, "PGSSLMODE=" + opts["sslmode"]}
	for _, k := range []string{"sslrootcert", "sslcert", "sslkey"} {
		if opts[k] != "" {
			env = append(env, "PG"+strings.ToUpper(k)+"="+opts[k])
		}
	}
	return env, nil
}
