package monitorlogs

import "time"

type RetentionRun struct {
	RunID          string    `json:"run_id"`
	TriggeredBy    string    `json:"triggered_by"`
	DryRun         bool      `json:"dry_run"`
	RetentionDays  int       `json:"retention_days"`
	CutoffAt       time.Time `json:"cutoff_at"`
	PreviewDetails []string  `json:"preview_details,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	EndedAt        time.Time `json:"ended_at"`
	Status         string    `json:"status"`
	DeletedFiles   int64     `json:"deleted_files"`
	DeletedRows    int64     `json:"deleted_rows"`
	Sources        []string  `json:"sources"`
	ErrorSummary   string    `json:"error_summary"`
	DurationMS     int64     `json:"duration_ms"`
}

type RetentionRunList struct {
	Items    []RetentionRun `json:"items"`
	NextRun  *time.Time     `json:"next_run,omitempty"`
	Enabled  bool           `json:"enabled"`
	Cron     string         `json:"cron"`
	Timezone string         `json:"timezone"`
}

type RetentionExportFile struct {
	Name      string `json:"name"`
	SizeBytes int    `json:"size_bytes"`
	Content   string `json:"content"`
	MimeType  string `json:"mime_type"`
}

type RetentionExport struct {
	RunID         string              `json:"run_id"`
	Format        string              `json:"format"`
	RetentionDays int                 `json:"retention_days"`
	CutoffAt      time.Time           `json:"cutoff_at"`
	MatchedFiles  int64               `json:"matched_files"`
	MatchedRows   int64               `json:"matched_rows"`
	PerTableRows  map[string]int64    `json:"per_table_rows"`
	Files         []string            `json:"files,omitempty"`
	Sources       []string            `json:"sources"`
	Errors        []string            `json:"errors,omitempty"`
	File          RetentionExportFile `json:"file"`
}

type RetentionPolicy struct {
	Enabled              bool                   `json:"enabled"`
	Cron                 string                 `json:"cron"`
	Timezone             string                 `json:"timezone"`
	DefaultRetentionDays int                    `json:"default_retention_days"`
	FilePaths            []string               `json:"file_paths"`
	BatchSize            int                    `json:"batch_size"`
	MaxDeleteRowsPerRun  int                    `json:"max_delete_rows_per_run"`
	DBTables             []RetentionDBTableView `json:"db_tables"`
}

type RetentionDBTableView struct {
	Name          string `json:"name"`
	TimeColumn    string `json:"time_column"`
	RetentionDays int    `json:"retention_days"`
}
