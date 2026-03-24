<template>
  <section class="mx-auto max-w-6xl space-y-6 p-6">
    <header class="space-y-1">
      <h1 class="text-2xl font-semibold">部署发布中心</h1>
      <p class="text-sm text-gray-500">支持 Docker / systemd 双模式发布与回滚，记录全链路状态。</p>
    </header>

    <div class="grid gap-4 md:grid-cols-3">
      <div class="rounded-lg border border-gray-200 bg-white p-4 md:col-span-2">
        <h2 class="mb-3 text-sm font-medium text-gray-700">发布参数</h2>
        <form class="grid gap-3 md:grid-cols-2" @submit.prevent="submitRelease">
          <label class="text-sm">
            <span class="mb-1 block text-gray-600">环境</span>
            <input v-model="form.environment" class="w-full rounded border border-gray-300 px-3 py-2" />
          </label>
          <label class="text-sm">
            <span class="mb-1 block text-gray-600">模式</span>
            <select v-model="form.mode" class="w-full rounded border border-gray-300 px-3 py-2">
              <option value="docker">docker</option>
              <option value="systemd">systemd</option>
            </select>
          </label>
          <label class="text-sm">
            <span class="mb-1 block text-gray-600">Backend 版本</span>
            <input v-model="form.backendVersion" class="w-full rounded border border-gray-300 px-3 py-2" />
          </label>
          <label class="text-sm">
            <span class="mb-1 block text-gray-600">Web Admin 版本</span>
            <input v-model="form.webAdminVersion" class="w-full rounded border border-gray-300 px-3 py-2" />
          </label>
          <div class="md:col-span-2">
            <button type="submit" class="rounded bg-black px-4 py-2 text-sm text-white" :disabled="loading">
              {{ loading ? "提交中..." : "触发发布" }}
            </button>
          </div>
        </form>
      </div>

      <div class="rounded-lg border border-gray-200 bg-white p-4">
        <h2 class="mb-2 text-sm font-medium text-gray-700">健康状态</h2>
        <p class="text-lg font-semibold">{{ health.status || "-" }}</p>
        <p class="mt-1 text-sm text-gray-500">{{ health.summary || "暂无状态" }}</p>
        <button class="mt-3 text-xs text-gray-700 underline" @click="refreshHealth">刷新</button>
      </div>
    </div>

    <div class="rounded-lg border border-gray-200 bg-white p-4">
      <div class="mb-3 flex items-center justify-between">
        <h2 class="text-sm font-medium text-gray-700">发布记录</h2>
        <button class="text-xs text-gray-700 underline" @click="loadReleases">刷新</button>
      </div>
      <div class="overflow-x-auto">
        <table class="min-w-full text-sm">
          <thead>
            <tr class="border-b border-gray-200 text-left text-gray-500">
              <th class="py-2">环境</th>
              <th class="py-2">动作</th>
              <th class="py-2">版本</th>
              <th class="py-2">状态</th>
              <th class="py-2">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in releases" :key="String(item.id)" class="border-b border-gray-100">
              <td class="py-2">{{ item.environment }}</td>
              <td class="py-2">{{ item.action }}</td>
              <td class="py-2">{{ item.backend_version }}</td>
              <td class="py-2">{{ item.status }}</td>
              <td class="py-2">
                <button
                  class="text-xs text-red-600 underline"
                  @click="submitRollback(item.environment, item.backend_version)"
                >
                  回滚到此版本
                </button>
              </td>
            </tr>
            <tr v-if="releases.length === 0">
              <td colspan="5" class="py-4 text-center text-gray-400">暂无记录</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { reactive, ref, onMounted } from "vue";
import { useDeployOpsService, type DeployHealthSummary, type DeployReleaseRecord } from "~/composables/api/services/deployOpsService";

const deploySvc = useDeployOpsService();
const loading = ref(false);
const releases = ref<DeployReleaseRecord[]>([]);
const health = ref<DeployHealthSummary>({ status: "", summary: "" });

const form = reactive({
  environment: "prod",
  mode: "docker" as "docker" | "systemd",
  backendVersion: "",
  webAdminVersion: "",
});

const loadReleases = async () => {
  const result = await deploySvc.listReleases({ environment: form.environment, page: 1, pageSize: 20 });
  releases.value = result.items;
};

const refreshHealth = async () => {
  health.value = await deploySvc.getHealth();
};

const submitRelease = async () => {
  loading.value = true;
  try {
    await deploySvc.triggerRelease(
      {
        environment: form.environment,
        backend_version: form.backendVersion,
        web_admin_version: form.webAdminVersion,
      },
      { mode: form.mode, approvalTickets: form.environment === "prod" ? 2 : 0 }
    );
    await Promise.all([loadReleases(), refreshHealth()]);
  } finally {
    loading.value = false;
  }
};

const submitRollback = async (environment: string, targetVersion: string) => {
  loading.value = true;
  try {
    await deploySvc.triggerRollback(
      {
        environment,
        target_version: targetVersion,
      },
      { mode: form.mode, approvalTickets: environment === "prod" ? 2 : 0 }
    );
    await Promise.all([loadReleases(), refreshHealth()]);
  } finally {
    loading.value = false;
  }
};

onMounted(async () => {
  await Promise.all([loadReleases(), refreshHealth()]);
});
</script>
