package system_recovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// 在线旧包没有来源行数合同，不能伪造冻结证明。只在本地开发演练中，
// 以成功 pg_restore 的数据库为校验基线；完整性仍由原包 SHA256 验证。
func onlineRestoredContract(ctx context.Context, dsn string, base captureContract) (captureContract, error) {
	c, e := inspectDatabase(ctx, dsn)
	if e != nil {
		return c, e
	}
	c.InstanceID = base.InstanceID
	c.Protocol = base.Protocol
	c.Platform = base.Platform
	c.Version = base.Version
	c.Environment = base.Environment
	c.RedisDatabase = base.RedisDatabase
	return c, nil
}
func (n *Native) saveRedisRecoveryPoint(ctx context.Context, d Deployment) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	raw, e := os.ReadFile(filepath.Join(d.Root, "redis", "dump.rdb"))
	if e != nil {
		return errors.New("独立 Redis 退出后未生成可恢复的本地持久点")
	}
	return os.WriteFile(filepath.Join(d.Root, "redis-recovery-point.rdb"), raw, 0600)
}
