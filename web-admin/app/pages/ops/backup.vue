<template>
  <section class="mx-auto max-w-6xl space-y-6 p-6">
    <header>
      <h1 class="text-2xl font-semibold">备份恢复中心</h1>
      <p class="mt-1 text-sm text-gray-500">策略管理、任务执行、恢复演练与日志观测。</p>
    </header>

    <div class="grid gap-4 md:grid-cols-2">
      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <h3 class="mb-3 text-sm font-medium text-gray-700">备份策略</h3>
        <div class="grid gap-2">
          <input v-model="policyForm.name" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="策略名称" />
          <input v-model="policyForm.backupType" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="backup_type" />
          <input v-model="policyForm.schedule" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="cron" />
          <input v-model.number="policyForm.retentionDays" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="retention days" />
          <input v-model="policyForm.storageTarget" class="rounded border border-gray-300 px-3 py-2 text-sm" placeholder="storage target" />
          <button class="rounded bg-black px-4 py-2 text-sm text-white" @click="savePolicy">保存策略</button>
        </div>
      </div>

      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <h3 class="mb-3 text-sm font-medium text-gray-700">执行动作</h3>
        <div class="space-y-2">
          <select v-model="selectedPolicyId" class="w-full rounded border border-gray-300 px-3 py-2 text-sm">
            <option value="">选择策略</option>
            <option v-for="p in policies" :key="String(p.id)" :value="String(p.id)">{{ p.name }}</option>
          </select>
          <button class="rounded border border-gray-300 px-4 py-2 text-sm" @click="runBackup">手动触发备份</button>
          <button class="rounded border border-gray-300 px-4 py-2 text-sm" @click="runCleanup">触发清理</button>
          <button class="rounded border border-gray-300 px-4 py-2 text-sm" :disabled="!latestJob" @click="runDrill">触发恢复演练</button>
        </div>
      </div>
    </div>

    <BackupJobTable :items="jobs" />

    <div class="grid gap-4 md:grid-cols-2">
      <RestoreDrillPanel :drill="latestDrill" />
      <LogObservabilityPanel />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useBackupOpsService, type BackupJob, type BackupPolicy, type RestoreDrillRecord } from "~/composables/api/services/backupOpsService";
import BackupJobTable from "~/components/ops/backup/BackupJobTable.vue";
import RestoreDrillPanel from "~/components/ops/backup/RestoreDrillPanel.vue";
import LogObservabilityPanel from "~/components/ops/backup/LogObservabilityPanel.vue";

const backupSvc = useBackupOpsService();
const policies = ref<BackupPolicy[]>([]);
const jobs = ref<BackupJob[]>([]);
const latestDrill = ref<RestoreDrillRecord | null>(null);
const selectedPolicyId = ref("");

const policyForm = reactive({
  name: "daily-main",
  backupType: "logical",
  schedule: "0 2 * * *",
  retentionDays: 30,
  storageTarget: "s3://powerx-backup/main",
});

const latestJob = computed(() => (jobs.value.length > 0 ? jobs.value[0] : null));

const load = async () => {
  policies.value = await backupSvc.listPolicies(false);
  jobs.value = await backupSvc.listJobs();
};

const savePolicy = async () => {
  await backupSvc.upsertPolicy({
    name: policyForm.name,
    backup_type: policyForm.backupType,
    schedule: policyForm.schedule,
    retention_days: policyForm.retentionDays,
    enabled: true,
    storage_target: policyForm.storageTarget,
  });
  await load();
};

const runBackup = async () => {
  if (!selectedPolicyId.value) return;
  await backupSvc.triggerJob(selectedPolicyId.value);
  jobs.value = await backupSvc.listJobs();
};

const runCleanup = async () => {
  await backupSvc.triggerCleanup();
};

const runDrill = async () => {
  if (!latestJob.value) return;
  latestDrill.value = await backupSvc.triggerRestoreDrill(latestJob.value.id);
};

onMounted(async () => {
  await load();
});
</script>
