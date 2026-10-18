package system_recovery

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/jackc/pgx/v5"
	"strings"
)

// 未纳入统一恢复的数据存储必须阻断，不能以“缓存”名义丢弃队列或外部数据。
func validateDependencies(ctx context.Context, cfg *config.Config, dsn string) error {
	if cfg.AI.Runtime.DurableSessions.Enabled {
		return errors.New("Agent 持久报告位于外部 S3，当前包尚未包含该对象存储，已阻断生产整站恢复；本地演练必须明确停用该功能")
	}
	address := fmt.Sprintf("%s:%d", cfg.Cache.Host, cfg.Cache.Port)
	for _, endpoint := range []string{cfg.Queue.Redis.Addr, cfg.Event.Bus.RedisAddr, cfg.Event.Fabric.RedisAddr, cfg.Event.Fabric.Authorization.RedisAddr, cfg.KnowledgeSpace.RedisAddr, cfg.IntegrationGateway.RedisAddr, cfg.AgentLifecycle.RedisAddr} {
		if endpoint != "" && endpoint != address {
			return errors.New("配置了另一台持久 Redis，当前完整恢复适配只支持独占的同一 Redis 实例；不同 DB 编号会保留")
		}
	}
	if cfg.Server.GRPC.UseTLS || cfg.Server.GRPC.TLS != nil {
		return errors.New("独立 gRPC TLS 的证书与调用方映射尚无适配，已阻断整站恢复采集")
	}
	if cfg.FeatureGate.EnableKnowledgeSpace {
		driver := strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.Driver)
		if driver != "" && driver != "pgvector" {
			return errors.New("外部知识存储没有恢复适配，已阻断整站恢复采集")
		}
		vector := cfg.KnowledgeSpace.VectorStore.PgVector.DSN
		if vector != "" && vector != dsn {
			return errors.New("独立知识数据库尚未纳入完整快照")
		}
		for _, index := range []string{cfg.KnowledgeSpace.IndexBackends.Sparse, cfg.KnowledgeSpace.IndexBackends.Hier, cfg.KnowledgeSpace.IndexBackends.StructuredFields} {
			if index == "external" {
				return errors.New("外部知识索引尚未纳入完整快照")
			}
		}
	}
	db, e := pgx.Connect(ctx, dsn)
	if e != nil {
		return errors.New("无法预检持久依赖")
	}
	defer db.Close(context.WithoutCancel(ctx))
	var foreign int
	if e = db.QueryRow(ctx, "SELECT count(*) FROM pg_foreign_server").Scan(&foreign); e != nil {
		return e
	}
	if foreign > 0 {
		return errors.New("检测到 PostgreSQL 外部服务器，无法保证断开源依赖恢复")
	}
	safe := map[string]bool{"plpgsql": true, "vector": true, "pg_trgm": true, "btree_gin": true, "btree_gist": true, "uuid-ossp": true, "pgcrypto": true, "hstore": true, "citext": true, "ltree": true, "unaccent": true, "tablefunc": true}
	extensions, e := db.Query(ctx, "SELECT extname FROM pg_extension ORDER BY extname")
	if e != nil {
		return e
	}
	defer extensions.Close()
	for extensions.Next() {
		var name string
		if e = extensions.Scan(&name); e != nil {
			return e
		}
		if !safe[name] {
			return fmt.Errorf("PostgreSQL 扩展 %s 没有经过恢复隔离适配", name)
		}
	}
	return extensions.Err()
}
