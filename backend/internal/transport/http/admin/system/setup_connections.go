package system

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
)

const (
	setupSourceInstance = "instance"
	setupSourceManual   = "manual"
)

type setupInstanceConnections struct {
	Database setupDatabaseConfig `json:"database"`
	Cache    setupCacheConfig    `json:"cache"`
}

func isContainerSetup() bool {
	return os.Getenv("POWERX_MODE") == "docker"
}

func setupInstanceAvailable() bool {
	cfg := config.GetGlobalConfig()
	return isContainerSetup() && cfg != nil && cfg.Install.EffectiveStatus() != "installed"
}

func readSetupInstanceConnections() *setupInstanceConnections {
	instance := &setupInstanceConnections{
		Database: setupDatabaseConfig{Source: setupSourceInstance, Type: "postgresql"},
		Cache:    setupCacheConfig{Source: setupSourceInstance, Type: "redis"},
	}
	path := resolveRuntimeConfigPath()
	if !setupInstanceAvailable() || path == "" {
		return instance
	}
	// Load 合并运行时覆盖，同时检查挂载文件，防止通用默认凭据被误认为实例凭据。
	raw, err := os.ReadFile(path)
	var mounted config.Config
	if err != nil || yaml.Unmarshal(raw, &mounted) != nil {
		return instance
	}
	cfg, err := config.Load(path)
	if err != nil {
		return instance
	}
	mountedDB, mountedCache := setupConnectionsFromConfig(&mounted)
	database, cache := setupConnectionsFromConfig(cfg)
	if mountedDB.Type == "postgresql" && validateDatabaseOnly(mountedDB) == nil && database.Type == "postgresql" {
		if mountedDB.Password == "" {
			database.Password = ""
		}
		dsn, err := buildDatabaseDSN("postgres", database)
		if err == nil && (cfg.Database.DSN == "" || cfg.Database.DSN == dsn) {
			database.Source = setupSourceInstance
			instance.Database = database
		}
	}
	if mountedCache.Type == "redis" && validateSetupRedis(mountedCache) == nil && cache.Type == "redis" {
		if mountedCache.RedisPass == "" {
			cache.RedisPass = ""
		}
		cache.Source = setupSourceInstance
		instance.Cache = cache
	}
	return instance
}

func setupConnectionsFromConfig(cfg *config.Config) (setupDatabaseConfig, setupCacheConfig) {
	db := cfg.Database
	return setupDatabaseConfig{
			Type: normalizeDBType(db.Driver), Host: db.Host, Port: db.Port,
			Name: db.Database, Username: db.UserName, Password: db.Password,
			SSLMode: db.SSLMode,
		}, setupCacheConfig{
			Type: cfg.Cache.Driver, RedisHost: cfg.Cache.Host, RedisPort: cfg.Cache.Port,
			RedisPass: cfg.Cache.Password, RedisDB: cfg.Cache.DB,
		}
}

func (in setupInstanceConnections) public() *setupInstanceConnections {
	in.Database = publicSetupDatabase(in.Database)
	in.Cache = publicSetupCache(in.Cache)
	return &in
}

func publicSetupDatabase(in setupDatabaseConfig) setupDatabaseConfig {
	in.PasswordConfigured = in.Password != ""
	in.Password = ""
	return in
}

func publicSetupCache(in setupCacheConfig) setupCacheConfig {
	in.PasswordConfigured = in.RedisPass != ""
	in.RedisPass = ""
	return in
}

func publicSetupConfig(in setupConfigPayload) setupConfigPayload {
	in.Database = publicSetupDatabase(in.Database)
	in.Cache = publicSetupCache(in.Cache)
	in.Admin.Password = ""
	in.HTTPS.KeyContent = ""
	in.Storage.SecretKey = ""
	in.LLM.APIKey = ""
	return in
}

func setupDraftForStorage(in setupConfigPayload) setupConfigPayload {
	if in.Database.Source == setupSourceInstance {
		in.Database = setupDatabaseConfig{Source: setupSourceInstance}
	}
	if in.Cache.Source == setupSourceInstance {
		in.Cache = setupCacheConfig{Source: setupSourceInstance}
	}
	return in
}

func resolveSetupConnections(in setupConfigPayload) (setupConfigPayload, error) {
	var instance *setupInstanceConnections
	if in.Database.Source == setupSourceInstance || in.Cache.Source == setupSourceInstance {
		instance = readSetupInstanceConnections()
	}
	var err error
	in.Database, err = resolveSetupDatabase(in.Database, instance)
	if err != nil {
		return in, err
	}
	in.Cache, err = resolveSetupCache(in.Cache, instance)
	return in, err
}

func resolveSetupDatabase(in setupDatabaseConfig, instance *setupInstanceConnections) (setupDatabaseConfig, error) {
	switch in.Source {
	case setupSourceInstance:
		if instance == nil {
			instance = readSetupInstanceConnections()
		}
		in = instance.Database
		if validateDatabaseOnly(in) != nil || in.Password == "" {
			return in, errors.New("setup.connection.instanceDatabaseUnavailable")
		}
	case "", setupSourceManual:
		in.Type = strings.ToLower(strings.TrimSpace(in.Type))
		if in.Type == "" {
			in.Type = "postgresql"
		}
		in.PasswordConfigured = false
		if isContainerSetup() && in.Type != "sqlite" && in.Password == "" {
			return in, errors.New("setup.connection.databasePasswordRequired")
		}
	default:
		return in, errors.New("setup.connection.invalidSource")
	}
	return in, validateDatabaseOnly(in)
}

func resolveSetupCache(in setupCacheConfig, instance *setupInstanceConnections) (setupCacheConfig, error) {
	switch in.Source {
	case setupSourceInstance:
		if instance == nil {
			instance = readSetupInstanceConnections()
		}
		in = instance.Cache
		if validateSetupRedis(in) != nil || in.RedisPass == "" {
			return in, errors.New("setup.connection.instanceCacheUnavailable")
		}
	case "", setupSourceManual:
		in.Type = strings.ToLower(strings.TrimSpace(in.Type))
		if in.Type == "" {
			in.Type = "redis"
		}
		in.PasswordConfigured = false
		if isContainerSetup() {
			if in.Type != "redis" {
				return in, errors.New("setup.connection.cacheDriverUnsupported")
			}
			if in.RedisPass == "" {
				return in, errors.New("setup.connection.cachePasswordRequired")
			}
		}
	default:
		return in, errors.New("setup.connection.invalidSource")
	}
	if in.Type == "redis" {
		return in, validateSetupRedis(in)
	}
	return in, nil
}

func validateSetupRedis(in setupCacheConfig) error {
	if strings.TrimSpace(in.RedisHost) == "" || in.RedisPort <= 0 || in.RedisPort > 65535 || in.RedisDB < 0 {
		return errors.New("setup.connection.cacheAddressInvalid")
	}
	return nil
}

func testSetupDatabase(ctx context.Context, in setupDatabaseConfig) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pingDatabase(ctx, in); err != nil {
		return errors.New("setup.connection.databaseCheckFailed")
	}
	return nil
}

func testSetupCache(ctx context.Context, in setupCacheConfig) error {
	if in.Type != "redis" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client := redis.NewClient(&redis.Options{
		Addr: setupRedisAddress(in.RedisHost, in.RedisPort), Password: in.RedisPass, DB: in.RedisDB,
		ContextTimeoutEnabled: true,
	})
	defer client.Close()
	if err := client.Ping(ctx).Err(); err != nil {
		detail := strings.ToUpper(err.Error())
		switch {
		case strings.Contains(detail, "WRONGPASS"), strings.Contains(detail, "INVALID PASSWORD"):
			return errors.New("setup.connection.redisAuthInvalid")
		case strings.Contains(detail, "NOAUTH"):
			return errors.New("setup.connection.redisAuthRequired")
		case strings.Contains(detail, "NOPERM"):
			return errors.New("setup.connection.redisPermissionDenied")
		}
		return errors.New("setup.connection.cacheCheckFailed")
	}
	return nil
}

func checkContainerSetupConnections(ctx context.Context, in setupConfigPayload) error {
	if !isContainerSetup() {
		return nil
	}
	if err := testSetupDatabase(ctx, in.Database); err != nil {
		return err
	}
	return testSetupCache(ctx, in.Cache)
}

func setupRedisAddress(host string, port int) string {
	return net.JoinHostPort(strings.TrimSpace(host), strconv.Itoa(port))
}

func applySetupCacheConfig(root map[string]any, in setupCacheConfig) {
	cache := asMap(root["cache"])
	root["cache"] = cache
	cache["driver"] = in.Type
	if in.Type != "redis" {
		return
	}
	oldHost, _ := cache["host"].(string)
	oldPassword, _ := cache["password"].(string)
	oldAddress := setupRedisAddress(oldHost, intFromAny(cache["port"]))
	newAddress := setupRedisAddress(in.RedisHost, in.RedisPort)
	cache["host"], cache["port"], cache["password"], cache["db"] = strings.TrimSpace(in.RedisHost), in.RedisPort, in.RedisPass, in.RedisDB

	// 容器模板把多处 Redis 配置指向同一实例。只迁移共享原连接的条目。
	update := func(entry map[string]any, addrKey, passKey string) {
		address, _ := entry[addrKey].(string)
		password, _ := entry[passKey].(string)
		if address == oldAddress && (password == oldPassword || password == "") {
			entry[addrKey], entry[passKey] = newAddress, in.RedisPass
		}
	}
	update(asMap(asMap(root["queue"])["redis"]), "addr", "password")
	event := asMap(root["event"])
	fabric := asMap(event["fabric"])
	for _, entry := range []map[string]any{
		asMap(event["bus"]), fabric, asMap(fabric["authorization"]),
		asMap(root["knowledge_space"]), asMap(root["integration_gateway"]), asMap(root["agent_lifecycle"]),
	} {
		update(entry, "redis_addr", "redis_password")
	}
}

func prepareSetupPhaseConfig(runtimePath string, original []byte, in setupConfigPayload) (string, error) {
	if isContainerSetup() {
		// 安装完成前保留挂载配置，重试或回退时仍可选用原实例连接。
		return stageSetupRuntimeConfig(runtimePath, original, in)
	}
	return runtimePath, writeRuntimeConfig(runtimePath, in, "configuring")
}

func writePrivateSetupFile(path string, contents []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".setup-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(contents)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func stageSetupRuntimeConfig(runtimePath string, original []byte, in setupConfigPayload) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(runtimePath), ".setup-runtime-*.yaml")
	if err != nil {
		return "", errors.New("setup.connection.runtimeWriteFailed")
	}
	path := file.Name()
	_, writeErr := file.Write(original)
	if errors.Join(writeErr, file.Close()) != nil || writeRuntimeConfig(path, in, "configuring") != nil {
		_ = os.Remove(path)
		return "", errors.New("setup.connection.runtimeWriteFailed")
	}
	if !setupRuntimeMatches(path, in) {
		_ = os.Remove(path)
		return "", errors.New("setup.connection.runtimeConflict")
	}
	return path, nil
}

func setupRuntimeMatches(path string, in setupConfigPayload) bool {
	cfg, err := config.Load(path)
	if err != nil {
		return false
	}
	expectedDSN := strings.TrimSpace(in.Database.SQLitePath)
	if in.Database.Type != "sqlite" {
		expectedDSN, err = buildDatabaseDSN(cfg.Database.Driver, in.Database)
		if err != nil {
			return false
		}
	}
	db, cache := setupConnectionsFromConfig(cfg)
	if db.Type != in.Database.Type || cfg.Database.DSN != expectedDSN ||
		(db.Type != "sqlite" && (db.Host != strings.TrimSpace(in.Database.Host) ||
			db.Port != in.Database.Port || db.Name != strings.TrimSpace(in.Database.Name) ||
			db.Username != strings.TrimSpace(in.Database.Username) || db.Password != in.Database.Password)) {
		return false
	}
	return cache.Type == in.Cache.Type && (cache.Type != "redis" ||
		(cache.RedisHost == strings.TrimSpace(in.Cache.RedisHost) && cache.RedisPort == in.Cache.RedisPort &&
			cache.RedisPass == in.Cache.RedisPass && cache.RedisDB == in.Cache.RedisDB))
}

func publicSetupProvisionError(err error) error {
	if isContainerSetup() {
		return nil
	}
	return err
}
