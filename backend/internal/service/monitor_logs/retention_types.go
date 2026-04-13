package monitorlogs

import "time"

type RetentionRun struct {
	RunID        string    `json:"run_id"`
	TriggeredBy  string    `json:"triggered_by"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	Status       string    `json:"status"`
	DeletedFiles int64     `json:"deleted_files"`
	DeletedRows  int64     `json:"deleted_rows"`
	Sources      []string  `json:"sources"`
	ErrorSummary string    `json:"error_summary"`
	DurationMS   int64     `json:"duration_ms"`
}

type RetentionRunList struct {
	Items    []RetentionRun `json:"items"`
	NextRun  *time.Time     `json:"next_run,omitempty"`
	Enabled  bool           `json:"enabled"`
	Cron     string         `json:"cron"`
	Timezone string         `json:"timezone"`
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
