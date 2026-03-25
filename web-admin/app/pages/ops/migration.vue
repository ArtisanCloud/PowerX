<template>
  <section class="mx-auto max-w-6xl space-y-6 p-6">
    <header>
      <h1 class="text-2xl font-semibold">实例迁移中心</h1>
      <p class="mt-1 text-sm text-gray-500">支持 A->B 迁移演练、验收确认、切换与回切流程。</p>
      <p v-if="permissionHint" class="text-xs text-amber-600">{{ permissionHint }}</p>
    </header>

    <div class="grid gap-4 md:grid-cols-2">
      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <h3 class="mb-3 text-sm font-medium text-gray-700">迁移运行</h3>
        <div class="grid gap-2">
          <input v-model="form.sourceEnv" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="source env" />
          <input v-model="form.targetEnv" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="target env" />
          <label class="inline-flex items-center gap-2 text-sm text-gray-600">
            <input v-model="form.dryRun" type="checkbox" />
            dry run
          </label>
          <button class="rounded bg-black px-4 py-2 text-sm text-white" :disabled="loading || !canExecute" @click="triggerMigration">
            {{ loading ? "执行中..." : "触发迁移" }}
          </button>
        </div>
      </div>

      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <h3 class="mb-3 text-sm font-medium text-gray-700">验收与切换</h3>
        <div class="grid gap-2">
          <input v-model="acceptanceForm.conclusion" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="验收结论" />
          <label class="inline-flex items-center gap-2 text-sm text-gray-600">
            <input v-model="acceptanceForm.dbMigrationCompleted" type="checkbox" />
            DB 迁移完成
          </label>
          <label class="inline-flex items-center gap-2 text-sm text-gray-600">
            <input v-model="acceptanceForm.instanceMigrationPassed" type="checkbox" />
            实例验收通过
          </label>
          <div class="flex gap-2">
            <button class="rounded border border-gray-300 px-4 py-2 text-sm" :disabled="!currentRecord || loading || !canExecute" @click="acceptMigration">提交验收</button>
            <button class="rounded border border-gray-300 px-4 py-2 text-sm" :disabled="!currentRecord || loading || !canExecute" @click="switchTraffic(false)">流量切换</button>
            <button class="rounded border border-gray-300 px-4 py-2 text-sm" :disabled="!currentRecord || loading || !canExecute" @click="switchTraffic(true)">回切</button>
          </div>
        </div>
      </div>
    </div>

    <div class="rounded-lg border border-gray-200 bg-white p-4" v-if="currentRecord">
      <h3 class="mb-3 text-sm font-medium text-gray-700">迁移记录</h3>
      <dl class="grid gap-y-2 text-sm md:grid-cols-2 md:gap-x-8">
        <div><dt class="text-gray-500">ID</dt><dd>{{ currentRecord.id }}</dd></div>
        <div><dt class="text-gray-500">状态</dt><dd>{{ currentRecord.status }}</dd></div>
        <div><dt class="text-gray-500">源环境</dt><dd>{{ currentRecord.source_env }}</dd></div>
        <div><dt class="text-gray-500">目标环境</dt><dd>{{ currentRecord.target_env }}</dd></div>
        <div><dt class="text-gray-500">DB 迁移</dt><dd>{{ currentRecord.db_migration_status }}</dd></div>
        <div><dt class="text-gray-500">实例验收</dt><dd>{{ currentRecord.instance_acceptance_status }}</dd></div>
        <div><dt class="text-gray-500">流量切换</dt><dd>{{ currentRecord.traffic_switch_status }}</dd></div>
        <div><dt class="text-gray-500">流量回切</dt><dd>{{ currentRecord.traffic_rollback_status }}</dd></div>
      </dl>
      <p class="mt-3 text-sm text-gray-500">摘要：{{ currentRecord.summary || "无" }}</p>
      <p v-if="lastOperationId" class="mt-1 text-xs text-gray-500">最近操作 ID：{{ lastOperationId }}</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { useMigrationOpsService, type MigrationRunbookRecord } from "~/composables/api/services/migrationOpsService";
import { useOpsAccess } from "~/composables/useOpsAccess";

const migrationSvc = useMigrationOpsService();
const { canExecute, permissionHint, loadUserContext } = useOpsAccess();
const loading = ref(false);
const currentRecord = ref<MigrationRunbookRecord | null>(null);
const lastOperationId = ref("");

const form = reactive({
  sourceEnv: "prod-a",
  targetEnv: "prod-b",
  dryRun: false,
});

const acceptanceForm = reactive({
  dbMigrationCompleted: true,
  instanceMigrationPassed: true,
  conclusion: "核心能力验收通过",
});

const triggerMigration = async () => {
  if (!canExecute.value) return;
  loading.value = true;
  try {
    currentRecord.value = await migrationSvc.triggerMigration({
      source_env: form.sourceEnv,
      target_env: form.targetEnv,
      dry_run: form.dryRun,
    });
    if (currentRecord.value) {
      currentRecord.value = await migrationSvc.getMigration(currentRecord.value.id);
    }
  } finally {
    loading.value = false;
  }
};

const acceptMigration = async () => {
  if (!canExecute.value) return;
  if (!currentRecord.value) return;
  loading.value = true;
  try {
    currentRecord.value = await migrationSvc.acceptMigration(currentRecord.value.id, {
      db_migration_completed: acceptanceForm.dbMigrationCompleted,
      instance_migration_passed: acceptanceForm.instanceMigrationPassed,
      conclusion: acceptanceForm.conclusion,
    });
  } finally {
    loading.value = false;
  }
};

const switchTraffic = async (rollback: boolean) => {
  if (!canExecute.value) return;
  if (!currentRecord.value) return;
  loading.value = true;
  try {
    const result = await migrationSvc.triggerTrafficSwitch({
      migration_id: String(currentRecord.value.id),
      rollback,
    });
    lastOperationId.value = result.operation_id;
    currentRecord.value = result.record;
  } finally {
    loading.value = false;
  }
};

onMounted(async () => {
  await loadUserContext();
});
</script>
