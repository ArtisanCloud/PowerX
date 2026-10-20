package archive_ops

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	medriver "github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	s3driver "github.com/ArtisanCloud/PowerX/internal/infra/media/driver/s3"
	"github.com/jackc/pgx/v5"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

func (s *Service) capture(ctx context.Context, j *jobRecord, p Policy, source Source) error {
	return s.captureWithMode(ctx, j, p, source, false)
}
func (s *Service) captureWithMode(ctx context.Context, j *jobRecord, p Policy, source Source, online bool) error {
	if containsPath(source.Directory, s.cfg.Directory) {
		return errors.New("归档目录不能位于来源目录内部，拒绝备份自身")
	}
	active := map[string]bool{}
	if source.Kind == "postgresql_logs" {
		raw, e := os.ReadFile(source.CurrentLogfiles)
		if e != nil {
			return errors.New("无法识别正在写入的 PostgreSQL 日志")
		}
		scanner := bufio.NewScanner(strings.NewReader(string(raw)))
		for scanner.Scan() {
			parts := strings.SplitN(scanner.Text(), " ", 2)
			if len(parts) != 2 {
				return errors.New("current_logfiles 格式无效")
			}
			active[filepath.Base(parts[1])] = true
		}
		if len(active) == 0 {
			return errors.New("current_logfiles 为空，拒绝猜测活动日志")
		}
	}
	entries := map[string]Entry{}
	// 媒体数据库事务快照保持打开，pg_dump 和媒体引用清单共享同一 MVCC 快照。
	var conn *pgx.Conn
	var tx pgx.Tx
	if p.IncludeDatabase {
		var e error
		conn, e = pgx.Connect(ctx, source.DatabaseDSN)
		if e != nil {
			return errors.New("无法连接媒体配套快照数据库")
		}
		defer conn.Close(context.Background())
		tx, e = conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if e != nil {
			return e
		}
		defer tx.Rollback(context.Background())
		if e = tx.QueryRow(ctx, "SELECT current_database(),pg_export_snapshot(),statement_timestamp()").Scan(&j.Database, &j.SnapshotID, &j.CreatedAt); e != nil {
			return errors.New("无法导出数据库事务快照")
		}
	}
	var e error
	if !online {
		e = filepath.WalkDir(source.Directory, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return fmt.Errorf("无法读取来源目录: %s", filepath.Base(path))
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("来源包含软链接，拒绝越界归档: %s", d.Name())
			}
			if d.IsDir() {
				return nil
			}
			if source.Kind == "postgresql_logs" {
				if active[d.Name()] {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(path))
				if ext != ".log" && ext != ".csv" && ext != ".json" && ext != ".gz" {
					return nil
				}
			}
			if source.Kind == "database_files" && !strings.HasSuffix(path, ".dump") && !strings.HasSuffix(path, ".dump.sha256") && !strings.HasSuffix(path, ".dump.json") {
				return nil
			}
			rel, e := filepath.Rel(source.Directory, path)
			if e != nil {
				return e
			}
			prefix := "media"
			if source.Kind != "media" {
				prefix = source.Kind
			}
			name := prefix + "/" + filepath.ToSlash(rel)
			entry, added, e := s.captureFile(ctx, path, name)
			if e != nil {
				return e
			}
			entries[name] = entry
			j.NewBytes += added
			if len(entries)%20 == 0 {
				j.FileCount = len(entries)
				j.Phase = "capturing"
				if e = s.saveJob(j); e != nil {
					return e
				}
			}
			return nil
		})
	}
	if e != nil {
		return e
	}
	if source.Kind == "postgresql_logs" && len(entries) == 0 {
		return errors.New("没有已轮转日志；活动日志不会归档为完整文件")
	}
	if tx != nil {
		if e = s.captureMediaReferences(ctx, tx, source, j, entries, online); e != nil {
			return e
		}
		if e = s.dumpSnapshot(ctx, source, j); e != nil {
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return errors.New("数据库快照事务中断")
		}
	}
	for _, entry := range entries {
		j.Files = append(j.Files, entry)
	}
	j.FileCount = len(j.Files)
	for _, f := range j.Files {
		j.SizeBytes += f.Size
	}
	if j.FileCount == 0 {
		return errors.New("来源没有可归档文件")
	}
	j.Phase = "verified"
	return s.saveJob(j)
}
func containsPath(root, child string) bool {
	rel, e := filepath.Rel(root, child)
	return e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func (s *Service) captureFile(ctx context.Context, path, name string) (Entry, int64, error) {
	var out Entry
	if !cleanEntryPath(name) {
		return out, 0, errors.New("文件相对路径无效")
	}
	before, e := os.Lstat(path)
	if e != nil || !before.Mode().IsRegular() {
		return out, 0, errors.New("来源文件不是普通文件")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return out, 0, errors.New("无法打开来源文件")
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(before, opened) {
		return out, 0, errors.New("来源文件身份变化")
	}
	out, added, e := s.captureReader(ctx, f, name, before.Size(), before.ModTime())
	if e != nil {
		return out, added, e
	}
	after, e := f.Stat()
	if e != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return out, added, errors.New("来源文件在读取中变化，快照失败，请重试")
	}
	return out, added, nil
}
func (s *Service) captureReader(ctx context.Context, r io.Reader, name string, size int64, modified time.Time) (Entry, int64, error) {
	out := Entry{Path: name, ModifiedAt: modified.UTC()}
	f, e := os.CreateTemp(filepath.Join(s.cfg.Directory, "staging"), "object-")
	if e != nil {
		return out, 0, e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	hash := sha256.New()
	n, e := io.Copy(io.MultiWriter(f, hash), &contextReader{ctx, r})
	if e == nil && size >= 0 && size != n {
		e = errors.New("文件大小与元数据不符")
	}
	if e == nil {
		e = f.Sync()
	}
	closed := f.Close()
	if e != nil {
		return out, 0, e
	}
	if closed != nil {
		return out, 0, closed
	}
	out.Size = n
	out.SHA256 = hex.EncodeToString(hash.Sum(nil))
	dest := s.objectPath(out.SHA256)
	if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
		return out, 0, e
	}
	if info, e := os.Lstat(dest); e == nil {
		if !info.Mode().IsRegular() || info.Size() != n {
			return out, 0, errors.New("已有增量对象损坏")
		}
		if e = verifyObject(ctx, dest, out); e != nil {
			return out, 0, e
		}
		return out, 0, nil
	}
	if e = os.Rename(tmp, dest); e != nil {
		return out, 0, e
	}
	return out, n, nil
}
func verifyObject(ctx context.Context, path string, entry Entry) error {
	if !shaPattern.MatchString(entry.SHA256) || entry.Size < 0 {
		return errors.New("对象清单格式无效")
	}
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return errors.New("归档对象不可读取")
	}
	defer f.Close()
	hash := sha256.New()
	n, e := io.Copy(hash, &contextReader{ctx, f})
	if e != nil {
		return e
	}
	if n != entry.Size || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
		return errors.New("归档对象 SHA256 或大小校验失败")
	}
	return nil
}
func (s *Service) captureMediaReferences(ctx context.Context, tx pgx.Tx, source Source, j *jobRecord, entries map[string]Entry, online bool) error {
	var s3 medriver.StorageDriver
	for _, table := range []string{"public.media_assets", "public.media_asset_variants"} {
		var exists *string
		if e := tx.QueryRow(ctx, "SELECT to_regclass($1)::text", table).Scan(&exists); e != nil {
			return e
		}
		if exists == nil {
			continue
		}
		rows, e := tx.Query(ctx, "SELECT driver,bucket,storage_key,size_bytes,COALESCE(expected_checksum,'') FROM "+table+" WHERE deleted_at IS NULL AND upload_state='ready'")
		if e != nil {
			return errors.New("媒体引用清单读取失败")
		}
		type ref struct {
			checksum            string
			driver, bucket, key string
			size                int64
		}
		refs := []ref{}
		for rows.Next() {
			var r ref
			if e = rows.Scan(&r.driver, &r.bucket, &r.key, &r.size, &r.checksum); e != nil {
				rows.Close()
				return e
			}
			refs = append(refs, r)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, r := range refs {
			joined := strings.Trim(strings.Join([]string{r.bucket, r.key}, "/"), "/")
			if !cleanEntryPath(joined) {
				return errors.New("数据库媒体引用路径无效")
			}
			name := "media/" + joined
			if online && (r.driver != "local" || !shaPattern.MatchString(r.checksum) || r.size < 0) {
				return errors.New("在线系统快照要求本地媒体、有效 SHA256 和大小；请修复媒体元数据")
			}
			if r.driver == "local" {
				if online {
					entry, added, err := s.captureReferencedFile(ctx, source.Directory, joined, r.size, r.checksum)
					if err != nil {
						return err
					}
					if prior, ok := entries[name]; ok && (prior.SHA256 != entry.SHA256 || prior.Size != entry.Size) {
						return errors.New("同一媒体路径存在不同内容引用，请修复媒体元数据")
					}
					entries[name] = entry
					j.NewBytes += added
				}
				if _, ok := entries[name]; !ok {
					entry, added, e := s.captureFile(ctx, filepath.Join(source.Directory, filepath.FromSlash(joined)), name)
					if e != nil {
						return fmt.Errorf("数据库引用的媒体文件缺失或变化: %s", joined)
					}
					entries[name] = entry
					j.NewBytes += added
				}
				entry := entries[name]
				if r.size > 0 && entry.Size != r.size {
					return fmt.Errorf("媒体大小与数据库快照不符: %s", joined)
				}
				if shaPattern.MatchString(r.checksum) && entry.SHA256 != r.checksum {
					return fmt.Errorf("媒体 SHA256 与数据库快照不符: %s", joined)
				}
				entry.Driver = "local"
				entry.Bucket = r.bucket
				entry.StorageKey = r.key
				entries[name] = entry
				continue
			}
			if r.driver != "s3" {
				return fmt.Errorf("不支持完整归档的媒体驱动: %s", r.driver)
			}
			if source.S3 == nil {
				return errors.New("媒体包含 S3 对象，但未配置该来源的 S3 读取凭据")
			}
			if s3 == nil {
				c := source.S3
				var e error
				s3, e = s3driver.New(s3driver.Options{Endpoint: c.Endpoint, Bucket: c.Bucket, AccessKey: c.AccessKey, SecretKey: c.SecretKey, Region: c.Region, UseSSL: c.UseSSL, ForcePathStyle: c.ForcePathStyle})
				if e != nil {
					return errors.New("S3 归档客户端配置无效")
				}
			}
			name = "s3/" + joined
			if _, ok := entries[name]; ok {
				continue
			}
			object, e := s3.Get(ctx, medriver.GetObjectInput{Bucket: r.bucket, ObjectKey: r.key})
			if e != nil {
				return errors.New("S3 媒体对象读取失败")
			}
			entry, added, e := s.captureReader(ctx, object.Body, name, object.Size, object.LastModified)
			object.Body.Close()
			if e != nil {
				return e
			}
			if r.size > 0 && entry.Size != r.size {
				return errors.New("S3 对象大小与数据库快照不符")
			}
			if shaPattern.MatchString(r.checksum) && entry.SHA256 != r.checksum {
				return errors.New("S3 对象 SHA256 与数据库快照不符")
			}
			entry.Driver = "s3"
			entry.Bucket = r.bucket
			entry.StorageKey = r.key
			entries[name] = entry
			j.NewBytes += added
		}
	}
	return nil
}
func (s *Service) dumpSnapshot(ctx context.Context, source Source, j *jobRecord) error {
	cfg, e := pgx.ParseConfig(source.DatabaseDSN)
	if e != nil {
		return errors.New("数据库快照连接配置无效")
	}
	tmp := filepath.Join(s.cfg.Directory, "staging", j.ID+".dump")
	defer os.Remove(tmp)
	cmd := exec.CommandContext(ctx, "pg_dump", "--no-password", "--format=custom", "--snapshot="+j.SnapshotID, "--file", tmp)
	env := append(os.Environ(), "PGHOST="+cfg.Host, "PGPORT="+fmt.Sprint(cfg.Port), "PGUSER="+cfg.User, "PGPASSWORD="+cfg.Password, "PGDATABASE="+cfg.Database)
	// URI 中的 TLS 参数保留，不把 DSN 或密码放入命令行。
	mode := "prefer"
	if cfg.TLSConfig == nil {
		mode = "disable"
	}
	if u, e := url.Parse(source.DatabaseDSN); e == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		if v := u.Query().Get("sslmode"); v != "" {
			mode = v
		}
		for _, key := range []string{"sslrootcert", "sslcert", "sslkey"} {
			if v := u.Query().Get(key); v != "" {
				env = append(env, "PG"+strings.ToUpper(key)+"="+v)
			}
		}
	} else {
		rx := regexp.MustCompile(`(?:^|\s)(sslmode|sslrootcert|sslcert|sslkey)=(?:'([^']*)'|([^\s]+))`)
		for _, m := range rx.FindAllStringSubmatch(source.DatabaseDSN, -1) {
			v := m[2]
			if v == "" {
				v = m[3]
			}
			if m[1] == "sslmode" {
				mode = v
			} else {
				env = append(env, "PG"+strings.ToUpper(m[1])+"="+v)
			}
		}
	}
	cmd.Env = append(env, "PGSSLMODE="+mode)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if e = cmd.Run(); e != nil {
		return errors.New("配套 pg_dump 快照失败，请检查客户端版本和读取权限")
	}
	entry, added, e := s.captureFile(ctx, tmp, "database/database.dump")
	if e != nil {
		return e
	}
	j.Files = append(j.Files, entry)
	j.NewBytes += added
	return nil
}
