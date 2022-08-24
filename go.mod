module github.com/ArtisanCloud/PowerX

go 1.17

replace github.com/ArtisanCloud/PowerWeChat/v2 => ../PowerWeChat

//
replace github.com/ArtisanCloud/PowerLibs/v2 => ../PowerLibs

//
//replace github.com/ArtisanCloud/PowerSocialite/v2 => ../PowerSocialite

require (
	github.com/ArtisanCloud/PowerLibs/v2 v2.0.36
	github.com/ArtisanCloud/PowerSocialite/v2 v2.0.13
	github.com/ArtisanCloud/PowerWeChat/v2 v2.0.1-beta36
	github.com/ArtisanCloud/ubt-go v1.0.6
	github.com/dgrijalva/jwt-go v3.2.0+incompatible
	github.com/gin-contrib/cors v1.3.1
	github.com/gin-gonic/gin v1.8.1
	github.com/golang-module/carbon v1.5.5
	github.com/google/uuid v1.3.0
	github.com/spf13/viper v1.3.2
	go.uber.org/zap v1.21.0
	golang.org/x/text v0.3.7
	gorm.io/datatypes v1.0.7
	gorm.io/driver/postgres v1.3.4
	gorm.io/gorm v1.23.6
)

require (
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/gin-contrib/sse v0.1.0 // indirect
	github.com/go-playground/locales v0.14.0 // indirect
	github.com/go-playground/universal-translator v0.18.0 // indirect
	github.com/go-playground/validator/v10 v10.10.0 // indirect
	github.com/go-redis/redis/v8 v8.11.0 // indirect
	github.com/go-sql-driver/mysql v1.6.0 // indirect
	github.com/gobuffalo/envy v1.7.0 // indirect
	github.com/gobuffalo/packd v0.3.0 // indirect
	github.com/gobuffalo/packr v1.30.1 // indirect
	github.com/goccy/go-json v0.9.7 // indirect
	github.com/guonaihong/gout v0.2.12 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/jackc/chunkreader/v2 v2.0.1 // indirect
	github.com/jackc/pgconn v1.11.0 // indirect
	github.com/jackc/pgio v1.0.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgproto3/v2 v2.2.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20200714003250-2b9c44734f2b // indirect
	github.com/jackc/pgtype v1.10.0 // indirect
	github.com/jackc/pgx/v4 v4.15.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/joho/godotenv v1.3.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/leodido/go-urn v1.2.1 // indirect
	github.com/magiconair/properties v1.8.0 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mitchellh/mapstructure v1.1.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/patrickmn/go-cache v2.1.0+incompatible // indirect
	github.com/pelletier/go-toml v1.2.0 // indirect
	github.com/pelletier/go-toml/v2 v2.0.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	github.com/spf13/afero v1.1.2 // indirect
	github.com/spf13/cast v1.3.0 // indirect
	github.com/spf13/jwalterweatherman v1.0.0 // indirect
	github.com/spf13/pflag v1.0.3 // indirect
	github.com/ugorji/go/codec v1.2.7 // indirect
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/multierr v1.6.0 // indirect
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292 // indirect
	golang.org/x/net v0.0.0-20211112202133-69e39bad7dc2 // indirect
	golang.org/x/sys v0.0.0-20210806184541-e5e7981a1069 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gorm.io/driver/mysql v1.3.2 // indirect
)
