package archive_ops

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

type CatalogFilter struct {
	Query    string    `json:"q"`
	SourceID string    `json:"source_id"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	Page     int       `json:"page"`
	PageSize int       `json:"page_size"`
}
type CatalogPage struct {
	Items      []Job `json:"items"`
	Pagination struct {
		Total    int `json:"total"`
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
		Pages    int `json:"pages"`
	} `json:"pagination"`
}
type LogPreview struct {
	Name      string `json:"name"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
	Verified  bool   `json:"verified"`
}

// Catalog 从持久任务清单检索，不依赖源文件仍然存在。
func (s *Service) Catalog(ctx context.Context, in CatalogFilter) (CatalogPage, error) {
	out := CatalogPage{Items: []Job{}}
	if err := s.check(ctx); err != nil {
		return out, err
	}
	if in.Page < 1 || in.Page > 1000000 || in.PageSize < 1 || in.PageSize > 100 || len(in.Query) > 256 || (!in.From.IsZero() && !in.To.IsZero() && in.From.After(in.To)) {
		return out, failure(400, "ARCHIVE_FILTER_INVALID", "归档检索条件无效")
	}
	ids, err := s.records("jobs")
	if err != nil {
		return out, err
	}
	rows := []Job{}
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		j, err := s.readJob(id)
		if err != nil {
			return out, err
		}
		if !isLogSource(j.Kind) || (in.SourceID != "" && j.SourceID != in.SourceID) || (!in.From.IsZero() && j.CreatedAt.Before(in.From)) || (!in.To.IsZero() && j.CreatedAt.After(in.To)) {
			continue
		}
		if q := strings.ToLower(in.Query); q != "" {
			match := strings.Contains(strings.ToLower(j.SourceFileName+" "+j.SourceID+" "+j.ID), q)
			for _, f := range j.Files {
				match = match || strings.Contains(strings.ToLower(f.Path), q)
			}
			if !match {
				continue
			}
		}
		copy := j.Job
		copy.Files = nil
		rows = append(rows, copy)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].CreatedAt.After(rows[j].CreatedAt)
	})
	out.Pagination.Total = len(rows)
	out.Pagination.Page = in.Page
	out.Pagination.PageSize = in.PageSize
	out.Pagination.Pages = (len(rows) + in.PageSize - 1) / in.PageSize
	start := (in.Page - 1) * in.PageSize
	if start < len(rows) {
		out.Items = rows[start:min(start+in.PageSize, len(rows))]
	}
	return out, nil
}
func (s *Service) PreviewLog(ctx context.Context, id, name string) (LogPreview, error) {
	out := LogPreview{Name: name}
	if err := s.check(ctx); err != nil {
		return out, err
	}
	j, err := s.readJob(id)
	if err != nil {
		return out, err
	}
	if !isLogSource(j.Kind) || j.RemoteState == "expired" || j.Status == "running" {
		return out, failure(409, "ARCHIVE_NOT_READY", "请选择已完成且仍保留的日志归档")
	}
	unlock, err := s.lock(ctx, j.PolicyID)
	if err != nil {
		return out, ErrBusy
	}
	defer unlock()
	var entry *Entry
	for _, f := range j.Files {
		if f.Path == name {
			copy := f
			entry = &copy
			break
		}
	}
	if entry == nil || !cleanEntryPath(entry.Path) {
		return out, failure(404, "ARCHIVE_FILE_NOT_FOUND", "归档中不存在所选文件")
	}
	if err = s.ensureObjects(ctx, j); err != nil {
		return out, err
	}
	if err = verifyObject(ctx, s.objectPath(entry.SHA256), *entry); err != nil {
		return out, err
	}
	f, err := os.Open(s.objectPath(entry.SHA256))
	if err != nil {
		return out, err
	}
	defer f.Close()
	var reader io.Reader = f
	if strings.HasSuffix(entry.Path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return out, errors.New("归档压缩格式无效")
		}
		defer gz.Close()
		reader = gz
	}
	raw, err := io.ReadAll(io.LimitReader(reader, (64<<10)+1))
	if err != nil {
		return out, errors.New("日志内容读取失败")
	}
	out.Truncated = len(raw) > 64<<10
	out.Text = strings.ToValidUTF8(string(raw[:min(len(raw), 64<<10)]), "�")
	out.Verified = true
	s.auditAction(ctx, "log_preview", id, 0)
	return out, nil
}
