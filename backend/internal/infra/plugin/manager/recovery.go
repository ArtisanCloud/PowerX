package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"gopkg.in/yaml.v3"
)

// RemapRecoveryBindings 只接受已还原到独立 PostgreSQL 实例的受信任配置。
// 复用正式绑定算法，先配置角色与 schema，再写入目标环境的 Host 配置。
func RemapRecoveryBindings(ctx context.Context, cfg *config.Config, sourceRedisAddress ...string) ([]string, error) {
	registry := NewJSONRegistry(cfg.Plugin.RegistryFile)
	if e := registry.Load(ctx); e != nil {
		return nil, e
	}
	// 恢复中的角色密码不得进入 SQL 日志；不改变调用方的全局日志配置。
	privateConfig := *cfg
	privateConfig.Database.LogLevel = "silent"
	m := &managerImpl{opts: Options{CoreConfig: &privateConfig}}
	roles := []string{}
	for id, plugin := range registry.mem.Plugins {
		section, e := m.buildDatabaseSection(id)
		if e != nil {
			return nil, e
		}
		roles = append(roles, section.User)
		for version, record := range plugin.Versions {
			if record.Manifest.Runtime.Kind == plugin_mgr.RuntimeKindRemote {
				return nil, fmt.Errorf("remote plugin %s is not supported by system recovery", id)
			}
			old := record.Paths.Root
			target := filepath.Join(cfg.Plugin.InstalledDir, id, version)
			fields := reflect.ValueOf(&record.Paths).Elem()
			for i := 0; i < fields.NumField(); i++ {
				field := fields.Field(i)
				value := field.String()
				if value == "" {
					continue
				}
				rel, e := filepath.Rel(old, value)
				if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					return nil, fmt.Errorf("plugin %s registered path escapes its version root", id)
				}
				field.SetString(filepath.Join(target, rel))
			}
			if record.HostConfig == nil {
				record.HostConfig = &plugin_mgr.HostConfig{}
			}
			host := record.HostConfig
			if host.Values == nil {
				host.Values = map[string]string{}
			}
			if host.Spec == nil {
				host.Spec = map[string]any{}
			}
			host.ValuesFile = record.Paths.HostValuesFile
			if host.ValuesFile == "" {
				host.ValuesFile = filepath.Join(target, "config", hostValuesFileName)
				record.Paths.HostValuesFile = host.ValuesFile
			}
			record.Paths.ConfigDir = filepath.Dir(host.ValuesFile)
			// The encrypted host-values component is the deployed credential source.
			// Registry metadata may retain an older client identity; never let that
			// metadata overwrite the restored private file. Core verifies it against IAM.
			if raw, err := os.ReadFile(host.ValuesFile); err == nil {
				if len(sourceRedisAddress) > 0 {
					if err = validateRecoveryRedisSource(raw, sourceRedisAddress[0]); err != nil {
						return nil, err
					}
				}
				if err = restoreSTSIdentity(host.Values, raw); err != nil {
					return nil, fmt.Errorf("plugin %s restored host-values is invalid", id)
				}
			} else if !os.IsNotExist(err) {
				return nil, err
			}

			// Keep the frozen source endpoint so plugin-owned Redis consumers can
			// prove that they use the captured instance before rebinding it.
			if host.Values["POWERX_RECOVERY_REDIS_SOURCE_ADDR"] == "" {
				host.Values["POWERX_RECOVERY_REDIS_SOURCE_ADDR"] = host.Values["POWERX_REDIS_ADDR"]
			}

			for _, prefix := range []string{"POWERX_DB_", "POWERX_GRPC_", "POWERX_STS_", "PX_GATEWAY_", "POWERX_SECURITY_JWT_", "POWERX_CTX_"} {
				for key := range host.Values {
					// The restored IAM database retains the credential hash. Preserve its
					// encrypted-package client identity, while all endpoints are rebound.
					if key == "POWERX_STS_CLIENT_ID" || key == "POWERX_STS_CLIENT_SECRET" || key == "POWERX_STS_TENANT_UUID" {
						continue
					}
					if strings.HasPrefix(key, prefix) {
						delete(host.Values, key)
					}
				}
			}
			for key, value := range map[string]string{"POWERX_DB_DSN": section.DSN, "POWERX_DB_USERNAME": section.User, "POWERX_DB_PASSWORD": section.Password, "POWERX_DB_SCHEMA": section.Schema, "POWERX_PLUGIN_DB_SCHEMA": section.Schema, "POWERX_DEPLOYMENT_ENV": cfg.Deployment.Env, "POWERX_PLUGIN_CONFIG_DIR": record.Paths.ConfigDir, "POWERX_PLUGIN_HOST_VALUES": host.ValuesFile, "POWERX_GRPC_UPSTREAM_ADDRESS": fmt.Sprintf("127.0.0.1:%d", cfg.Server.GRPC.Port), "POWERX_REDIS_ADDR": fmt.Sprintf("127.0.0.1:%d", cfg.Cache.Port), "POWERX_REDIS_PASSWORD": cfg.Cache.Password, "POWERX_REDIS_DB": strconv.Itoa(cfg.Cache.DB), "POWERX_RECOVERY_READ_ONLY": strconv.FormatBool(cfg.Recovery.ReadOnly())} {
				host.Values[key] = value
			}
			host.Spec["database"] = map[string]any{"driver": section.Driver, "dsn": section.DSN, "schema": section.Schema, "user": section.User, "password": section.Password, "search_path": section.SearchPath, "managed": true, "deployment_env": section.DeploymentEnv, "plugin_key": section.PluginKey, "plugin_uuid": section.PluginUUID, "binding_uuid": section.BindingUUID}
			setNestedValue(host.Spec, []string{"deployment", "env"}, cfg.Deployment.Env)
			audience := "plugin:" + id
			for key, value := range map[string]string{"POWERX_SECURITY_MODE": "jwt", "POWERX_SECURITY_JWT_SECRET": cfg.Auth.JWTSecret, "POWERX_SECURITY_JWT_ISSUER": cfg.Auth.Issuer, "POWERX_SECURITY_JWT_AUDIENCE": audience, "POWERX_SECURITY_JWT_SCOPE": "access", "POWERX_SECURITY_CTX_HMAC_SECRET": cfg.Auth.JWTSecret, "PLUGIN_CTX_HMAC_SECRET": cfg.Auth.JWTSecret, "POWERX_CTX_ISSUER": cfg.Auth.Issuer, "POWERX_CTX_AUDIENCE": audience, "POWERX_CTX_TTL": cfg.Auth.AccessTTLStr} {
				host.Values[key] = value
			}
			setNestedValue(host.Spec, []string{"security", "mode"}, "jwt")
			for key, value := range map[string]any{"secret": cfg.Auth.JWTSecret, "issuer": cfg.Auth.Issuer, "audience": audience, "scope": "access"} {
				setNestedValue(host.Spec, []string{"security", "jwt", key}, value)
			}
			setNestedValue(host.Spec, []string{"security", "ctx_hmac", "secret"}, cfg.Auth.JWTSecret)
			setNestedValue(host.Spec, []string{"context", "hmac_secret"}, cfg.Auth.JWTSecret)
			host.Spec["env"] = host.Values
			delete(host.Values, "POWERX_HTTP_ADDR")
			delete(host.Values, "POWERX_GRPC_ADDR")

			setNestedValue(host.Spec, []string{"runtime", "run_migrate"}, !cfg.Recovery.ReadOnly())
			setNestedValue(host.Spec, []string{"redis", "addr"}, fmt.Sprintf("127.0.0.1:%d", cfg.Cache.Port))
			setNestedValue(host.Spec, []string{"redis", "password"}, cfg.Cache.Password)
			if e = os.MkdirAll(record.Paths.ConfigDir, 0700); e != nil {
				return nil, e
			}
			raw, e := yaml.Marshal(host.Spec)
			if e != nil {
				return nil, e
			}
			if e = os.WriteFile(host.ValuesFile, raw, 0600); e != nil {
				return nil, e
			}
			plugin.Versions[version] = record
		}
		registry.mem.Plugins[id] = plugin
	}
	// Save 与正常插件生命周期一致，以私有临时文件原子替换 registry。
	raw, e := json.MarshalIndent(registry.mem, "", "  ")
	if e != nil {
		return nil, e
	}
	if e = os.WriteFile(cfg.Plugin.RegistryFile+".recovery.tmp", raw, 0600); e != nil {
		return nil, e
	}
	if e = os.Rename(cfg.Plugin.RegistryFile+".recovery.tmp", cfg.Plugin.RegistryFile); e != nil {
		return nil, e
	}
	return roles, nil
}

// SetRecoveryRuntimeMode 不轮换数据库凭据，只在启停业务写入时同步 Host 运行模式。
func SetRecoveryRuntimeMode(ctx context.Context, cfg *config.Config, readOnly bool) error {
	registry := NewJSONRegistry(cfg.Plugin.RegistryFile)
	if e := registry.Load(ctx); e != nil {
		return e
	}
	for id, plugin := range registry.mem.Plugins {
		for version, record := range plugin.Versions {
			host := record.HostConfig
			if host == nil {
				continue
			}
			if host.Values == nil {
				host.Values = map[string]string{}
			}
			if host.Spec == nil {
				host.Spec = map[string]any{}
			}
			host.Values["POWERX_RECOVERY_READ_ONLY"] = strconv.FormatBool(readOnly)
			host.Values["POWERX_DISABLE_BACKGROUND_WORKERS"] = strconv.FormatBool(readOnly)
			setNestedValue(host.Spec, []string{"runtime", "run_migrate"}, !readOnly)
			setNestedValue(host.Spec, []string{"runtime", "recovery_read_only"}, readOnly)
			host.Spec["env"] = host.Values
			if host.ValuesFile != "" {
				raw, e := yaml.Marshal(host.Spec)
				if e != nil {
					return e
				}
				if e = os.WriteFile(host.ValuesFile, raw, 0600); e != nil {
					return e
				}
			}
			plugin.Versions[version] = record
		}
		registry.mem.Plugins[id] = plugin
	}
	raw, e := json.MarshalIndent(registry.mem, "", "  ")
	if e != nil {
		return e
	}
	file := cfg.Plugin.RegistryFile + ".recovery.tmp"
	if e = os.WriteFile(file, raw, 0600); e != nil {
		return e
	}
	return os.Rename(file, cfg.Plugin.RegistryFile)
}

// Restore only the deployed STS identity from the private package component.
// Endpoint/database/security values are always derived from the new target.
func restoreSTSIdentity(target map[string]string, raw []byte) error {
	var saved struct {
		Env map[string]string `yaml:"env"`
	}
	if err := yaml.Unmarshal(raw, &saved); err != nil {
		return err
	}
	if origin := saved.Env["POWERX_RECOVERY_REDIS_SOURCE_ADDR"]; origin != "" {
		target["POWERX_RECOVERY_REDIS_SOURCE_ADDR"] = origin
	} else if origin := saved.Env["POWERX_REDIS_ADDR"]; origin != "" {
		target["POWERX_RECOVERY_REDIS_SOURCE_ADDR"] = origin
	}
	for _, key := range []string{"POWERX_STS_CLIENT_ID", "POWERX_STS_CLIENT_SECRET", "POWERX_STS_TENANT_UUID"} {
		if value, ok := saved.Env[key]; ok {
			target[key] = value
		}
	}
	return nil
}

// Check the deployed current binding against the frozen Core instance. A saved
// logical origin may differ after an earlier restore, but the current endpoint
// must still be the same Redis that was captured with Core.
func validateRecoveryRedisSource(raw []byte, expected string) error {
	var saved struct {
		Env map[string]string `yaml:"env"`
	}
	if err := yaml.Unmarshal(raw, &saved); err != nil {
		return err
	}
	actual := saved.Env["POWERX_REDIS_ADDR"]
	if actual == "" {
		return nil
	}
	normalize := func(addr string) string {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return ""
		}
		host = strings.ToLower(host)
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			host = "loopback"
		}
		return net.JoinHostPort(host, port)
	}
	if normalize(actual) == "" || normalize(actual) != normalize(expected) {
		return fmt.Errorf("RECOVERY_PLUGIN_REDIS_EXTERNAL_UNSUPPORTED")
	}
	return nil
}
