package seed

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
)

func SeedDemoSkillInstallTasks(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}

	now := time.Now().UTC()
	started := now.Add(-15 * time.Minute)
	finished := now.Add(-14 * time.Minute)
	task := &skillmodel.SkillInstallTask{
		TaskID:      "seed-install-hello-echo-v1",
		TenantUUID:  "00000000-0000-0000-0000-000000000000",
		Provider:    "github",
		Repo:        "artisancloud/powerx-skill-examples",
		RepoURL:     "https://github.com/ArtisanCloud/powerx-skill-examples.git",
		SourcePath:  "skills/hello-echo",
		Ref:         "main",
		Method:      "auto",
		Source:      skillmodel.SkillSourceThirdParty,
		SkillID:     "skill.thirdparty.hello-echo",
		Version:     "1.0.0",
		InstallPath: "$CODEX_HOME/skills/hello-echo",
		Status:      skillmodel.SkillInstallStatusSuccess,
		StdoutLog:   "seeded install task for demo",
		StderrLog:   "",
		RequestedBy: "seed",
		StartedAt:   &started,
		FinishedAt:  &finished,
	}
	task.Normalize()
	hasTenantUUID := db.Migrator().HasColumn(&skillmodel.SkillInstallTask{}, "tenant_uuid")

	assignments := map[string]interface{}{
		"provider":      task.Provider,
		"repo":          task.Repo,
		"repo_url":      task.RepoURL,
		"source_path":   task.SourcePath,
		"source_ref":    task.Ref,
		"method":        task.Method,
		"source":        task.Source,
		"skill_id":      task.SkillID,
		"version":       task.Version,
		"install_path":  task.InstallPath,
		"status":        task.Status,
		"stdout_log":    task.StdoutLog,
		"stderr_log":    task.StderrLog,
		"requested_by":  task.RequestedBy,
		"started_at":    task.StartedAt,
		"finished_at":   task.FinishedAt,
		"error_summary": "",
		"updated_at":    now,
	}
	if hasTenantUUID {
		assignments["tenant_uuid"] = task.TenantUUID
	}

	tx := db.WithContext(seedCtx())
	if !hasTenantUUID {
		tx = tx.Omit("tenant_uuid")
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "task_id"}},
		DoUpdates: clause.Assignments(assignments),
	}).Create(task).Error; err != nil {
		return fmt.Errorf("upsert demo skill install task failed: %w", err)
	}

	fmt.Println("[seed] demo install task ready: seed-install-hello-echo-v1")
	return nil
}
