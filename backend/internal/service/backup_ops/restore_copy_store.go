package backup_ops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pxcrypto "github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
)

// RestoreCopy 是保留恢复库的操作清单，不包含源库或副本的密码。
type RestoreCopy struct {
	ID              string           `json:"id"`
	SourceJobID     uint64           `json:"source_job_id,omitempty"`
	ImportID        string           `json:"import_id,omitempty"`
	SourceName      string           `json:"source_name"`
	SourceSHA256    string           `json:"source_sha256"`
	Host            string           `json:"host"`
	Port            uint16           `json:"port"`
	CurrentDatabase string           `json:"current_database"`
	Database        string           `json:"database"`
	Status          string           `json:"status"`
	Phase           string           `json:"phase"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	Operator        string           `json:"operator"`
	TraceID         string           `json:"trace_id"`
	TableCount      int64            `json:"table_count"`
	SizeBytes       int64            `json:"size_bytes"`
	SchemaCounts    map[string]int64 `json:"schema_counts,omitempty"`
	BusinessCounts  map[string]int64 `json:"business_counts,omitempty"`
	Warnings        []string         `json:"warnings"`
	ErrorCode       string           `json:"error_code,omitempty"`
	ErrorMessage    string           `json:"error_message,omitempty"`
}

type copyLedger struct {
	RestoreCopy
	SourceKey        string            `json:"source_key"`
	IdempotencyHash  string            `json:"idempotency_hash"`
	DatabaseOID      uint32            `json:"database_oid"`
	OwnerRole        string            `json:"owner_role"`
	OwnerRoleOID     uint32            `json:"owner_role_oid"`
	OwnerPassword    *pxcrypto.Wrapped `json:"owner_password,omitempty"`
	ReaderRole       string            `json:"reader_role"`
	ReaderRoleOID    uint32            `json:"reader_role_oid"`
	ReaderExpiresAt  time.Time         `json:"reader_expires_at,omitempty"`
	AllowOlderServer bool              `json:"allow_older_server"`
}

type RestoreImport struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
	SourceKey string    `json:"-"`
}

type importLedger struct {
	RestoreImport
	Binding string `json:"source_key"`
}

type restoreCopyStore struct{ root string }

func restoreFailure(status int, code string, message string) error {
	return dto.NewErrorWithCode(status, code, message, nil)
}

func restoreSourceKey(host string, port uint16, database string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d/%s", host, port, database)))
	return hex.EncodeToString(sum[:])
}

func newRestoreCopyStore(root, sourceKey string) (*restoreCopyStore, error) {
	if root == "" {
		if links := os.Getenv("POWERX_LINKS_ROOT"); links != "" {
			root = filepath.Join(links, "storage", "restore-copies")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			root = filepath.Join(home, ".powerx", "storage", "restore-copies")
		}
	}
	if !filepath.IsAbs(root) {
		return nil, errors.New("restore directory must be absolute")
	}
	root = filepath.Join(root, sourceKey)
	if err := os.MkdirAll(filepath.Join(root, "imports"), 0700); err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	return &restoreCopyStore{root: canonical}, nil
}

func validCopyID(id string) bool {
	value, err := uuid.Parse(id)
	return err == nil && value != uuid.Nil && value.String() == id
}

func (s *restoreCopyStore) read(id string) (copyLedger, error) {
	var record copyLedger
	if !validCopyID(id) {
		return record, restoreFailure(400, "RESTORE_COPY_ID_INVALID", "恢复副本标识无效")
	}
	path := filepath.Join(s.root, id+".json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return record, restoreFailure(404, "RESTORE_COPY_NOT_FOUND", "恢复副本不存在")
	}
	if err != nil || !info.Mode().IsRegular() {
		return record, restoreFailure(503, "RESTORE_STORE_UNAVAILABLE", "恢复清单不可读取")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if json.Unmarshal(raw, &record) != nil || record.ID != id {
		return record, restoreFailure(503, "RESTORE_STORE_INVALID", "恢复清单损坏")
	}
	return record, nil
}

func (s *restoreCopyStore) save(record copyLedger) error {
	if !validCopyID(record.ID) {
		return errors.New("invalid copy ID")
	}
	record.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return atomicRestoreJSON(filepath.Join(s.root, record.ID+".json"), raw)
}

func atomicRestoreJSON(path string, raw []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".restore-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}

func (s *restoreCopyStore) list() ([]copyLedger, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	rows := []copyLedger{}
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !strings.HasSuffix(entry.Name(), ".json") || !validCopyID(id) {
			continue
		}
		row, err := s.read(id)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	return rows, nil
}
