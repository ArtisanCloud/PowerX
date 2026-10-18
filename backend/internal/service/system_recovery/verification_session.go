package system_recovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	glog "gorm.io/gorm/logger"
)

// The Core plugin identity contract requires a real live IAM session, not only
// a valid JWT signature. Only a private, owned candidate may receive this short
// verifier session. No refresh JWT is issued and no source session is changed.
func prepareVerificationSession(ctx context.Context, d Deployment, claims *reqctx.CoreXClaims) (func() error, error) {
	if claims == nil || !claims.IsRoot || uuid.Validate(claims.UserUUID) != nil || uuid.Validate(claims.TenantUUID) != nil || (claims.MemberUUID != "" && uuid.Validate(claims.MemberUUID) != nil) {
		return nil, errors.New("候选验证必须使用已存在的有效 Root 身份")
	}
	if d.SourceManaged {
		return nil, errors.New("来源实例验证需要现有的正常登录会话，不能创建隔离验证会话")
	}
	file := d.ConfigFile
	if file == "" {
		file = filepath.Join(d.Root, "runtime.yaml")
	}
	if file != filepath.Join(d.Root, "runtime.yaml") && file != filepath.Join(d.Root, "etc/config.yaml") {
		return nil, errors.New("验证会话配置必须属于登记实例")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var owner config.Config
	if err = yaml.Unmarshal(raw, &owner); err != nil {
		return nil, err
	}
	if err = validateVerificationDatabase(d, owner.Database.DSN); err != nil {
		return nil, err
	}
	db, err := gorm.Open(postgres.Open(owner.Database.DSN), &gorm.Config{Logger: glog.Default.LogMode(glog.Silent)})
	if err != nil {
		return nil, errors.New("无法登记候选验证会话")
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	rt := &models.RefreshToken{JTI: uuid.NewString(), UserUUID: claims.UserUUID, MemberUUID: claims.MemberUUID, TenantUUID: claims.TenantUUID, ExpiresAt: time.Now().Add(2 * time.Minute).UnixMilli(), IP: "127.0.0.1", UserAgent: "powerx-recovery-verifier/v1"}
	sessions := repo.NewRefreshTokenRepository(db)
	if err = sessions.Issue(ctx, rt); err != nil {
		_ = pool.Close()
		return nil, errors.New("无法登记候选验证会话")
	}
	claims.ID = rt.JTI
	return func() error {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := sessions.RevokeByJTI(cleanup, rt.JTI, time.Now().UnixMilli())
		if err == nil {
			// Remove only the row created above, preserving the frozen IAM session
			// inventory and every original login. Revocation happens first.
			removed := db.WithContext(cleanup).Unscoped().Where("id = ? AND jti = ? AND user_agent = ?", rt.ID, rt.JTI, rt.UserAgent).Delete(&models.RefreshToken{})
			if removed.Error != nil || removed.RowsAffected != 1 {
				err = errors.New("verification session cleanup failed")
			}
		}
		closeErr := pool.Close()
		if err != nil {
			return errors.New("候选验证会话撤销失败，等待短时凭据到期后再继续")
		}
		return closeErr
	}, nil
}

func validateVerificationDatabase(d Deployment, dsn string) error {
	invalid := errors.New("验证会话只允许写入工具拥有的独立候选数据库")
	if d.SourceManaged || uuid.Validate(d.ID) != nil || !filepath.IsAbs(d.Root) || d.SocketDirectory != filepath.Join(d.Root, "s") || d.PostgresData != filepath.Join(d.Root, "postgres") {
		return invalid
	}
	root, err := filepath.EvalSymlinks(d.Root)
	if err != nil || root != d.Root {
		return invalid
	}
	socket, err := filepath.EvalSymlinks(d.SocketDirectory)
	if err != nil || socket != d.SocketDirectory {
		return invalid
	}
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil || parsed.Host != d.SocketDirectory || int(parsed.Port) != d.PostgresPort || parsed.User != "powerx_recovery" || parsed.Database != "powerx" {
		return invalid
	}
	return nil
}
