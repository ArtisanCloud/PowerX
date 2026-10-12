// Package archive_ops 管理媒体快照及日志异地归档；清单独立于业务数据库。
package archive_ops

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"io"
	"time"
)

type Config struct {
	Directory   string                `yaml:"directory" json:"directory"`
	Instance    string                `yaml:"instance" json:"instance"`
	Sources     []Source              `yaml:"sources" json:"sources"`
	Credentials map[string]Credential `yaml:"credentials" json:"-"`
}
type Source struct {
	ID              string    `yaml:"id" json:"id"`
	Name            string    `yaml:"name" json:"name"`
	Kind            string    `yaml:"kind" json:"kind"`
	Directory       string    `yaml:"directory" json:"directory"`
	CurrentLogfiles string    `yaml:"current_logfiles" json:"current_logfiles,omitempty"`
	DatabaseDSN     string    `yaml:"database_dsn" json:"-"`
	Ready           bool      `yaml:"-" json:"ready"`
	Problem         string    `yaml:"-" json:"problem,omitempty"`
	S3              *S3Config `yaml:"s3,omitempty" json:"-"`
}
type S3Config struct {
	Endpoint       string `yaml:"endpoint"`
	Bucket         string `yaml:"bucket"`
	Region         string `yaml:"region"`
	AccessKey      string `yaml:"access_key"`
	SecretKey      string `yaml:"secret_key"`
	UseSSL         bool   `yaml:"use_ssl"`
	ForcePathStyle bool   `yaml:"force_path_style"`
}
type Credential struct {
	KeyFile        string `yaml:"key_file"`
	KnownHostsFile string `yaml:"known_hosts_file"`
}
type Policy struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	SourceID             string    `json:"source_id"`
	IntervalMinutes      int       `json:"interval_minutes"`
	RetentionDays        int       `json:"retention_days"`
	RetentionCount       int       `json:"retention_count"`
	RemoteRetentionDays  int       `json:"remote_retention_days"`
	RemoteRetentionCount int       `json:"remote_retention_count"`
	TargetID             string    `json:"target_id,omitempty"`
	IncludeDatabase      bool      `json:"include_database"`
	Enabled              bool      `json:"enabled"`
	CreatedAt            time.Time `json:"created_at"`
}
type Target struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	User          string `json:"user"`
	Directory     string `json:"directory"`
	CredentialRef string `json:"credential_ref"`
}
type Entry struct {
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
	Driver     string    `json:"driver,omitempty"`
	Bucket     string    `json:"bucket,omitempty"`
	StorageKey string    `json:"storage_key,omitempty"`
}
type Job struct {
	ID                   string    `json:"id"`
	PolicyID             string    `json:"policy_id"`
	SourceID             string    `json:"source_id"`
	Kind                 string    `json:"kind"`
	Status               string    `json:"status"`
	Phase                string    `json:"phase"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	Files                []Entry   `json:"files,omitempty"`
	FileCount            int       `json:"file_count"`
	SizeBytes            int64     `json:"size_bytes"`
	NewBytes             int64     `json:"new_bytes"`
	Database             string    `json:"database,omitempty"`
	SnapshotID           string    `json:"snapshot_id,omitempty"`
	Error                string    `json:"error,omitempty"`
	Warnings             []string  `json:"warnings"`
	Protected            bool      `json:"protected"`
	RemoteState          string    `json:"remote_state"`
	RemotePath           string    `json:"remote_path,omitempty"`
	RemoteError          string    `json:"remote_error,omitempty"`
	Attempts             int       `json:"attempts"`
	RetryAt              time.Time `json:"retry_at,omitempty"`
	Target               *Target   `json:"target,omitempty"`
	RemoteRetentionDays  int       `json:"remote_retention_days"`
	RemoteRetentionCount int       `json:"remote_retention_count"`
	Operator             string    `json:"operator"`
	TraceID              string    `json:"trace_id,omitempty"`
	IdempotencyHash      string    `json:"-"`
}
type jobRecord struct {
	Job
	RequestKey string `json:"request_key"`
}
type Restore struct {
	ID               string    `json:"id"`
	JobID            string    `json:"job_id"`
	Status           string    `json:"status"`
	Directory        string    `json:"directory"`
	CreatedAt        time.Time `json:"created_at"`
	Error            string    `json:"error,omitempty"`
	DatabaseCopyID   string    `json:"database_copy_id,omitempty"`
	DatabaseCopyName string    `json:"database_copy_name,omitempty"`
	FileCount        int       `json:"file_count"`
	DirectoryDevice  uint64    `json:"directory_device,omitempty"`
	DirectoryInode   uint64    `json:"directory_inode,omitempty"`
}
type Overview struct {
	Instance         string    `json:"instance"`
	Directory        string    `json:"directory"`
	Sources          []Source  `json:"sources"`
	Credentials      []string  `json:"credentials"`
	Policies         []Policy  `json:"policies"`
	Targets          []Target  `json:"targets"`
	Jobs             []Job     `json:"jobs"`
	Restores         []Restore `json:"restores"`
	PendingTransfers int       `json:"pending_transfers"`
}
type DatabaseRestorer func(context.Context, string, bool, string, string) (string, string, error)
type Options struct {
	RestoreDatabase DatabaseRestorer
	ConfigError     error
}

func failure(status int, code, msg string) error { return dto.NewErrorWithCode(status, code, msg, nil) }
func requireRoot(ctx context.Context) error {
	if !reqctx.IsRoot(ctx) {
		return failure(403, "ARCHIVE_ROOT_REQUIRED", "整站媒体及数据库日志归档仅允许 Root 管理员")
	}
	if c := reqctx.GetClaims(ctx); c != nil {
		if c.Issuer == "powerx-sts" {
			return failure(403, "ARCHIVE_USER_REQUIRED", "需要管理员用户会话")
		}
		for _, p := range c.Platforms {
			if p == "api_key" {
				return failure(403, "ARCHIVE_USER_REQUIRED", "API Key 不能管理整站归档")
			}
		}
	}
	return nil
}

var ErrBusy = errors.New("archive operation is busy")

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
