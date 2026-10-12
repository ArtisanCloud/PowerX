package backup_ops

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	pxcrypto "github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/pbkdf2"
)

// 密码仅以 SCRAM verifier 写入角色 DDL，避免日志记录明文密码。
func restoreSCRAM(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := pbkdf2.Key([]byte(password), salt, 4096, 32, sha256.New)
	h := hmac.New(sha256.New, key)
	h.Write([]byte("Client Key"))
	stored := sha256.Sum256(h.Sum(nil))
	h = hmac.New(sha256.New, key)
	h.Write([]byte("Server Key"))
	server := h.Sum(nil)
	return fmt.Sprintf("SCRAM-SHA-256$4096:%s$%s:%s", base64.StdEncoding.EncodeToString(salt), base64.StdEncoding.EncodeToString(stored[:]), base64.StdEncoding.EncodeToString(server)), nil
}

func (s *RestoreCopyService) execute(row copyLedger, archive restoreArchive) {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
	defer cancel()
	err := s.restore(ctx, &row, archive)
	if err != nil {
		s.fail(row, err)
		return
	}
	row.Status = "success"
	row.Phase = "completed"
	row.OwnerPassword = nil
	if err = s.persist(row); err != nil {
		s.fail(row, errors.New("无法保存完成报告，请检查运维目录"))
	}
}

func (s *RestoreCopyService) restore(ctx context.Context, row *copyLedger, archive restoreArchive) error {
	if err := s.phase(row, "validating"); err != nil {
		return err
	}
	artifact := &modelops.BackupArtifact{SizeBytes: archive.Size, Checksum: archive.Checksum}
	if err := verifyArtifactFile(archive.Path, artifact); err != nil {
		return errors.New("归档大小或 SHA256 校验失败")
	}
	header, err := readRestoreArchive(ctx, archive.Path)
	if err != nil {
		return err
	}
	if err = s.phase(row, "creating"); err != nil {
		return err
	}
	passwordBytes, err := pxcrypto.UnwrapPrivateKey(row.OwnerPassword)
	if err != nil {
		return errors.New("副本凭据无法解密")
	}
	password := string(passwordBytes)
	admin, err := s.connect(ctx, s.cfg.Database, s.cfg.User, s.cfg.Password)
	if err != nil {
		return errors.New("无法连接当前 PostgreSQL")
	}
	defer admin.Close(context.Background())
	verifier, err := restoreSCRAM(password)
	if err != nil {
		return err
	}
	if _, err = admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{row.OwnerRole}.Sanitize()+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOINHERIT PASSWORD "+restoreSQLLiteral(verifier)); err != nil {
		return errors.New("创建副本专用恢复账号失败；没有覆盖现有角色")
	}
	if err = admin.QueryRow(ctx, "SELECT oid FROM pg_roles WHERE rolname=$1", row.OwnerRole).Scan(&row.OwnerRoleOID); err != nil {
		return err
	}
	if err = s.persist(*row); err != nil {
		return err
	}
	if _, err = admin.Exec(ctx, "GRANT "+pgx.Identifier{row.OwnerRole}.Sanitize()+" TO "+pgx.Identifier{s.cfg.User}.Sanitize()); err != nil {
		return errors.New("管理员无法管理本次新建角色")
	}
	if _, err = admin.Exec(ctx, "CREATE ROLE "+pgx.Identifier{row.ReaderRole}.Sanitize()+" NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOINHERIT"); err != nil {
		return errors.New("创建副本只读账号失败")
	}
	if err = admin.QueryRow(ctx, "SELECT oid FROM pg_roles WHERE rolname=$1", row.ReaderRole).Scan(&row.ReaderRoleOID); err != nil {
		return err
	}
	if err = s.persist(*row); err != nil {
		return err
	}
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{row.Database}.Sanitize()+" OWNER "+pgx.Identifier{row.OwnerRole}.Sanitize()+" TEMPLATE template0"); err != nil {
		return errors.New("新库创建失败；没有删除或复用任何已有数据库")
	}
	if err = admin.QueryRow(ctx, "SELECT oid FROM pg_database WHERE datname=$1", row.Database).Scan(&row.DatabaseOID); err != nil {
		return err
	}
	if err = s.persist(*row); err != nil {
		return err
	}
	prepare, err := s.connect(ctx, row.Database, s.cfg.User, s.cfg.Password)
	if err != nil {
		return errors.New("无法准备新恢复库")
	}
	defer prepare.Close(context.Background())
	if _, err = prepare.Exec(ctx, "REVOKE TEMPORARY ON DATABASE "+pgx.Identifier{row.Database}.Sanitize()+" FROM PUBLIC"); err != nil {
		return err
	}
	if _, err = prepare.Exec(ctx, "REVOKE CREATE ON SCHEMA public FROM PUBLIC"); err != nil {
		return err
	}
	if _, err = prepare.Exec(ctx, "GRANT USAGE, CREATE ON SCHEMA public TO "+pgx.Identifier{row.OwnerRole}.Sanitize()); err != nil {
		return err
	}
	for _, extension := range header.extensions {
		if !allowedRestoreExtension(extension) {
			return fmt.Errorf("扩展 %s 未获准自动安装", extension)
		}
		if _, err = prepare.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+pgx.Identifier{extension}.Sanitize()); err != nil {
			return fmt.Errorf("准备扩展 %s 失败，请检查目标服务器权限", extension)
		}
	}
	prepare.Close(ctx)
	if err = s.phase(row, "restoring"); err != nil {
		return err
	}
	var serverVersion string
	if err = admin.QueryRow(ctx, "SHOW server_version").Scan(&serverVersion); err != nil {
		return err
	}
	serverMajor, _ := strconv.Atoi(strings.Split(serverVersion, ".")[0])
	if err = streamRestoreArchive(ctx, header.tool, archive.Path, s, row.Database, row.OwnerRole, password, serverMajor); err != nil {
		return err
	}
	if err = s.phase(row, "checking"); err != nil {
		return err
	}
	check, err := s.connect(ctx, row.Database, s.cfg.User, s.cfg.Password)
	if err != nil {
		return errors.New("无法检查恢复结果")
	}
	defer check.Close(context.Background())
	row.SchemaCounts = map[string]int64{}
	row.BusinessCounts = map[string]int64{}
	rows, err := check.Query(ctx, "SELECT schemaname,count(*) FROM pg_catalog.pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema') GROUP BY schemaname ORDER BY schemaname")
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		var count int64
		if err = rows.Scan(&name, &count); err != nil {
			rows.Close()
			return err
		}
		row.SchemaCounts[name] = count
		row.TableCount += count
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if row.TableCount == 0 {
		return errors.New("归档已导入，但没有业务表；请检查是否选错备份")
	}
	checks := map[string]string{"tenants": "public.iam_tenant", "users": "public.iam_user", "design_tasks": "px_com_powerx_plugins_ai_craft.ai_craft_design_tasks", "orders": "px_com_powerx_plugins_ai_craft.ai_craft_orders", "sample_rounds": "px_com_powerx_plugins_ai_craft.ai_craft_sample_rounds"}
	for label, qualified := range checks {
		var relation *string
		if err = check.QueryRow(ctx, "SELECT to_regclass($1)::text", qualified).Scan(&relation); err != nil {
			return err
		}
		if relation == nil {
			continue
		}
		parts := strings.Split(qualified, ".")
		var count int64
		if err = check.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier(parts).Sanitize()).Scan(&count); err != nil {
			return err
		}
		row.BusinessCounts[label] = count
	}
	if _, exists := row.SchemaCounts["public"]; exists {
		var relation *string
		_ = check.QueryRow(ctx, "SELECT to_regclass('public.metadata_dictionary_namespaces')::text").Scan(&relation)
		if relation != nil {
			var categories int64
			if err = check.QueryRow(ctx, "SELECT count(*) FROM public.metadata_dictionary_namespaces WHERE namespace='corex.agent.category' AND status='enabled' AND deleted_at IS NULL").Scan(&categories); err != nil {
				return err
			}
			if categories == 0 {
				row.Warnings = append(row.Warnings, "归档缺少启用的 corex.agent.category 字典；当前版本的智能体页面需要补齐基础数据")
			}
		}
	}
	if err = check.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&row.SizeBytes); err != nil {
		return err
	}
	for schema := range row.SchemaCounts {
		quoted := pgx.Identifier{schema}.Sanitize()
		reader := pgx.Identifier{row.ReaderRole}.Sanitize()
		for _, query := range []string{"REVOKE CREATE ON SCHEMA " + quoted + " FROM PUBLIC", "GRANT USAGE ON SCHEMA " + quoted + " TO " + reader, "GRANT SELECT ON ALL TABLES IN SCHEMA " + quoted + " TO " + reader, "GRANT SELECT ON ALL SEQUENCES IN SCHEMA " + quoted + " TO " + reader, "REVOKE EXECUTE ON ALL FUNCTIONS IN SCHEMA " + quoted + " FROM PUBLIC"} {
			if _, err = check.Exec(ctx, query); err != nil {
				return errors.New("配置只读核验权限失败")
			}
		}
	}
	if _, err = admin.Exec(ctx, "ALTER ROLE "+pgx.Identifier{row.OwnerRole}.Sanitize()+" NOLOGIN"); err != nil {
		return err
	}
	return nil
}

func restoreCommand(ctx context.Context, command string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// 生成 SQL 流时只去掉旧服务器不支持的 PG17 会话超时设置；表结构和数据不做转换。
func streamRestoreArchive(ctx context.Context, tool, path string, s *RestoreCopyService, database, user, password string, serverMajor int) error {
	psql, err := exec.LookPath("psql")
	if err != nil {
		return errors.New("缺少 psql 客户端")
	}
	decode := restoreCommand(ctx, tool, "--no-owner", "--no-privileges", "--no-comments", "--file=-", path)
	apply := restoreCommand(ctx, psql, "-X", "--single-transaction", "-v", "ON_ERROR_STOP=1", "-v", "VERBOSITY=verbose", "-d", database, "-U", user, "-h", s.cfg.Host, "-p", strconv.Itoa(int(s.cfg.Port)))
	env, err := postgresEnvironment(sourceDSN(s.db))
	if err != nil {
		return err
	}
	for i, value := range env {
		if strings.HasPrefix(value, "PGPASSWORD=") {
			env[i] = "PGPASSWORD=" + password
		}
		if strings.HasPrefix(value, "PGUSER=") {
			env[i] = "PGUSER=" + user
		}
		if strings.HasPrefix(value, "PGDATABASE=") {
			env[i] = "PGDATABASE=" + database
		}
	}
	apply.Env = append(apply.Environ(), env...)
	input, err := decode.StdoutPipe()
	if err != nil {
		return err
	}
	output, err := apply.StdinPipe()
	if err != nil {
		return err
	}
	decode.Stderr = io.Discard
	var diagnostics restoreDiagnostics
	apply.Stderr = &diagnostics
	apply.Stdout = io.Discard
	if err = apply.Start(); err != nil {
		return errors.New("无法启动新库导入进程")
	}
	if err = decode.Start(); err != nil {
		_ = syscall.Kill(-apply.Process.Pid, syscall.SIGTERM)
		output.Close()
		apply.Wait()
		return errors.New("无法启动归档读取进程")
	}
	reader := bufio.NewReader(input)
	var transferErr error
	copyData := false
	for lineNumber := 0; ; lineNumber++ {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			if copyData {
				if bytes.Equal(trimmed, []byte("\\.")) {
					copyData = false
				}
			} else {
				if bytes.HasPrefix(trimmed, []byte("\\restrict ")) || bytes.HasPrefix(trimmed, []byte("\\unrestrict ")) {
					continue
				}
				if bytes.HasPrefix(trimmed, []byte("\\")) {
					transferErr = errors.New("归档包含禁止的 psql 命令")
					break
				}
				if bytes.HasPrefix(trimmed, []byte("COPY ")) && bytes.HasSuffix(trimmed, []byte(" FROM stdin;")) {
					copyData = true
				}
			}
			if !(lineNumber < 100 && serverMajor < 17 && bytes.Equal(line, []byte("SET transaction_timeout = 0;\n"))) {
				if _, transferErr = output.Write(line); transferErr != nil {
					break
				}
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				transferErr = readErr
			}
			break
		}
	}
	if transferErr != nil && decode.Process != nil {
		_ = syscall.Kill(-decode.Process.Pid, syscall.SIGTERM)
	}
	decodeErr := decode.Wait()
	// 归档读取异常时先关闭导入进程，不能让 EOF 被 psql 当成正常结束并提交半份数据。
	if transferErr != nil || decodeErr != nil {
		_ = syscall.Kill(-apply.Process.Pid, syscall.SIGTERM)
	}
	output.Close()
	applyErr := apply.Wait()
	if transferErr != nil || decodeErr != nil || applyErr != nil {
		if state := regexp.MustCompile(`ERROR:\s+([A-Z0-9]{5}):`).FindSubmatch(diagnostics.data); len(state) > 1 {
			return fmt.Errorf("归档 SQL 导入失败（SQLSTATE %s）；请检查版本、扩展或对象兼容性", state[1])
		}
		return errors.New("归档 SQL 导入失败；请检查归档版本、扩展或对象兼容性")
	}
	return nil
}

// 仅保留有界诊断以提取 SQLSTATE，不返回 SQL、数据行或密码。
type restoreDiagnostics struct{ data []byte }

func (d *restoreDiagnostics) Write(p []byte) (int, error) {
	if available := 8192 - len(d.data); available > 0 {
		d.data = append(d.data, p[:min(len(p), available)]...)
	}
	return len(p), nil
}
