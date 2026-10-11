package backup_ops

import (
	"context"
	"encoding/base64"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	pxcrypto "github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRestoreCopyNativePostgres(t *testing.T) {
	dsn := os.Getenv("POWERX_RESTORE_TEST_ADMIN_DSN")
	if dsn == "" {
		t.Skip("opt-in native PostgreSQL restore acceptance")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	defer admin.Close(ctx)
	id := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	sourceName := "powerx_restore_test_" + id
	_, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{sourceName}.Sanitize()+" TEMPLATE template0")
	require.NoError(t, err)
	defer admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{sourceName}.Sanitize())
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/" + sourceName
	source, err := pgx.Connect(ctx, u.String())
	require.NoError(t, err)
	_, err = source.Exec(ctx, "CREATE TABLE sentinel(id integer PRIMARY KEY,value text); INSERT INTO sentinel VALUES(1,'original survives'); CREATE FUNCTION public.write_attempt() RETURNS void LANGUAGE sql SECURITY DEFINER AS $$INSERT INTO sentinel VALUES(2,'forbidden')$$")
	require.NoError(t, err)
	source.Close(ctx)
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	defer func() { pool, _ := db.DB(); pool.Close() }()
	oldKey := pxcrypto.GetGlobalKeyB64()
	require.NoError(t, pxcrypto.SetGlobalKeyB64(base64.StdEncoding.EncodeToString(make([]byte, 32))))
	defer pxcrypto.SetGlobalKeyB64(oldKey)
	s := NewRestoreCopyService(ctx, db, RestoreCopyOptions{Directory: t.TempDir()})
	defer s.Close()
	root := reqctx.WithIsRoot(ctx, true)
	file := t.TempDir() + "/source.dump"
	cmd := exec.Command("pg_dump", "--format=custom", "--file", file)
	env, err := postgresEnvironment(u.String())
	require.NoError(t, err)
	cmd.Env = append(os.Environ(), env...)
	require.NoError(t, cmd.Run())
	f, err := os.Open(file)
	require.NoError(t, err)
	imported, err := s.Import(root, "source.dump", f)
	f.Close()
	require.NoError(t, err)
	plan, err := s.Plan(root, RestoreSource{ImportID: imported.ID})
	require.NoError(t, err)
	require.True(t, plan.Ready)
	request := CreateRestoreCopy{RestoreSource: RestoreSource{ImportID: imported.ID}, IdempotencyKey: uuid.NewString(), Operator: "test.root", TraceID: uuid.NewString()}
	copy, err := s.Create(root, request)
	require.NoError(t, err)
	defer func() { err := s.Delete(root, copy.ID, copy.Database); require.NoError(t, err) }()
	same, err := s.Create(root, request)
	require.NoError(t, err)
	require.Equal(t, copy.ID, same.ID)
	conflict := request
	conflict.AllowOlderServer = true
	_, err = s.Create(root, conflict)
	require.Equal(t, 409, dto.StatusCode(err))
	require.Eventually(t, func() bool {
		current, e := s.Get(root, copy.ID)
		return e == nil && (current.Status == "success" || current.Status == "failed")
	}, 20*time.Second, 100*time.Millisecond)
	current, err := s.Get(root, copy.ID)
	require.NoError(t, err)
	require.Equal(t, "success", current.Status, current.ErrorMessage)
	require.EqualValues(t, 1, current.TableCount)
	// 文件清单库名相同但 OID/角色改变时，不能给凭据或删除。
	s.mu.Lock()
	identity, err := s.managed(copy.ID)
	require.NoError(t, err)
	changed := identity
	changed.DatabaseOID++
	require.NoError(t, s.store.save(changed))
	s.mu.Unlock()
	require.Equal(t, 409, dto.StatusCode(s.Delete(root, copy.ID, copy.Database)))
	s.mu.Lock()
	require.NoError(t, s.store.save(identity))
	changed = identity
	changed.ReaderRoleOID++
	require.NoError(t, s.store.save(changed))
	s.mu.Unlock()
	_, err = s.Connection(root, copy.ID)
	require.Equal(t, 409, dto.StatusCode(err))
	s.mu.Lock()
	require.NoError(t, s.store.save(identity))
	s.mu.Unlock()
	connection, err := s.Connection(root, copy.ID)
	require.NoError(t, err)
	require.True(t, connection.ReadOnly)
	require.NotEqual(t, s.cfg.User, connection.Username)
	reader, err := s.connect(ctx, connection.Database, connection.Username, connection.Password)
	require.NoError(t, err)
	var count int
	require.NoError(t, reader.QueryRow(ctx, "SELECT count(*) FROM sentinel").Scan(&count))
	require.Equal(t, 1, count)
	_, err = reader.Exec(ctx, "INSERT INTO sentinel VALUES(2,'changed')")
	require.Error(t, err)
	_, err = reader.Exec(ctx, "CREATE TABLE forbidden(id int)")
	require.Error(t, err)
	_, err = reader.Exec(ctx, "SELECT public.write_attempt()")
	require.Error(t, err)
	err = s.Delete(root, copy.ID, copy.Database)
	require.Equal(t, 409, dto.StatusCode(err))
	reader.Close(ctx)
	// trust 认证也必须由到期 NOLOGIN 阻止新登录，不能只依赖密码 VALID UNTIL。
	s.mu.Lock()
	ledger, err := s.managed(copy.ID)
	require.NoError(t, err)
	ledger.ReaderExpiresAt = time.Now().Add(-time.Minute)
	require.NoError(t, s.store.save(ledger))
	s.mu.Unlock()
	s.revokeExpiredReaders()
	var login bool
	require.NoError(t, admin.QueryRow(ctx, "SELECT rolcanlogin FROM pg_roles WHERE rolname=$1", connection.Username).Scan(&login))
	require.False(t, login)
	err = s.Delete(root, copy.ID, sourceName)
	require.Equal(t, 400, dto.StatusCode(err))
	unchanged, err := pgx.Connect(ctx, u.String())
	require.NoError(t, err)
	require.NoError(t, unchanged.QueryRow(ctx, "SELECT count(*) FROM sentinel WHERE value='original survives'").Scan(&count))
	require.Equal(t, 1, count)
	unchanged.Close(ctx)
	// 归档解码失败或出现 psql 导航命令时不允许 EOF 提交半份结构。
	for _, tail := range []string{"exit 1", "printf '\\\\connect postgres\\n'"} {
		decoder := t.TempDir() + "/decoder"
		script := "#!/bin/sh\nprintf 'CREATE TABLE rollback_probe(id integer);\\n'\n" + tail + "\n"
		require.NoError(t, os.WriteFile(decoder, []byte(script), 0700))
		err := streamRestoreArchive(ctx, decoder, file, s, sourceName, s.cfg.User, s.cfg.Password, 14)
		require.Error(t, err)
		probe, err := s.connect(ctx, sourceName, s.cfg.User, s.cfg.Password)
		require.NoError(t, err)
		var relation *string
		require.NoError(t, probe.QueryRow(ctx, "SELECT to_regclass('rollback_probe')::text").Scan(&relation))
		require.Nil(t, relation)
		probe.Close(ctx)
	}
	t.Logf("native source=%s copy=%s tables=%d read-only checks passed; owned databases cleaned up", sourceName, copy.Database, current.TableCount)
}
