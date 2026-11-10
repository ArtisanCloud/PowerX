package ai

import (
	"os"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	media "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ai"
	history "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/stretchr/testify/require"
)

func TestMediaSubmissionLegacyTaskHistoryMigrationAndSameKeyRecovery(t *testing.T) {
	if os.Getenv("POWERX_AI_MEDIA_TEST_DSN") == "" {
		t.Skip("requires disposable native PostgreSQL")
	}
	f := newMediaFixture(t)
	// 去掉新增结构，复现已安装旧库；本测试不启动模型消费。
	require.NoError(t, f.db.Exec("DROP INDEX "+core.PowerXSchema+".uidx_event_task_histories_idempotency").Error)
	require.NoError(t, f.db.Migrator().DropColumn(&history.TaskHistory{}, "idempotency_key"))
	input := videoInput()
	input.IdempotencyKey = "legacy-history-retry"
	task, err := f.s.Submit(f.ctx, input)
	require.Error(t, err)
	require.Nil(t, task)
	for _, model := range []interface{}{&media.MediaTask{}, &history.TaskHistory{}} {
		var count int64
		require.NoError(t, f.db.Model(model).Count(&count).Error)
		require.Zero(t, count, "task and history must roll back together")
	}
	require.Zero(t, f.generations.Load())
	require.NoError(t, database.MigrateTaskHistoryIdempotency(f.db))
	require.NoError(t, database.CheckTaskHistorySchema(f.db))
	accepted, err := f.s.Submit(f.ctx, input)
	require.NoError(t, err)
	require.Equal(t, "queued", accepted.Status)
	repeated, err := f.s.Submit(f.ctx, input)
	require.NoError(t, err)
	require.Equal(t, accepted.TaskUUID, repeated.TaskUUID)
	for _, model := range []interface{}{&media.MediaTask{}, &history.TaskHistory{}} {
		var count int64
		require.NoError(t, f.db.Model(model).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
	require.Zero(t, f.generations.Load(), "registration/recovery must not invoke the provider")
}
