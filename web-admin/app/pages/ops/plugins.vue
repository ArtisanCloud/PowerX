<template>
  <section class="mx-auto max-w-5xl space-y-6 p-6">
    <header>
      <h1 class="text-2xl font-semibold">插件生命周期中心</h1>
      <p class="mt-1 text-sm text-gray-500">支持插件版本切换、回滚与审计时间线查看。</p>
      <p v-if="permissionHint" class="text-xs text-amber-600">{{ permissionHint }}</p>
    </header>

    <div class="grid gap-4 rounded-lg border border-gray-200 bg-white p-4 md:grid-cols-4">
      <label class="text-sm md:col-span-1">
        <span class="mb-1 block text-gray-600">Plugin ID</span>
        <input v-model="form.pluginId" class="w-full rounded border border-gray-300 px-3 py-2" />
      </label>
      <label class="text-sm md:col-span-1">
        <span class="mb-1 block text-gray-600">动作</span>
        <select v-model="form.action" class="w-full rounded border border-gray-300 px-3 py-2">
          <option value="switch">switch</option>
          <option value="rollback">rollback</option>
        </select>
      </label>
      <label class="text-sm md:col-span-1">
        <span class="mb-1 block text-gray-600">From Version</span>
        <input v-model="form.fromVersion" class="w-full rounded border border-gray-300 px-3 py-2" />
      </label>
      <label class="text-sm md:col-span-1">
        <span class="mb-1 block text-gray-600">To Version</span>
        <input v-model="form.toVersion" class="w-full rounded border border-gray-300 px-3 py-2" />
      </label>
      <label class="text-sm md:col-span-4">
        <span class="mb-1 block text-gray-600">原因</span>
        <input v-model="form.reason" class="w-full rounded border border-gray-300 px-3 py-2" />
      </label>
      <div class="md:col-span-4 flex gap-2">
        <button class="rounded bg-black px-4 py-2 text-sm text-white" :disabled="loading || !canExecute" @click="triggerAction">
          {{ loading ? "执行中..." : "触发动作" }}
        </button>
        <button class="rounded border border-gray-300 px-4 py-2 text-sm" :disabled="loading" @click="loadAudits">刷新审计</button>
      </div>
    </div>

    <PluginAuditTimeline :items="audits" />
  </section>
</template>

<script setup lang="ts">
import { reactive, ref, onMounted } from "vue";
import PluginAuditTimeline from "~/components/ops/plugins/PluginAuditTimeline.vue";
import { usePluginOpsService, type PluginLifecycleAuditRecord } from "~/composables/api/services/pluginOpsService";
import { useOpsAccess } from "~/composables/useOpsAccess";

const pluginOpsService = usePluginOpsService();
const { canExecute, permissionHint, loadUserContext } = useOpsAccess();
const loading = ref(false);
const audits = ref<PluginLifecycleAuditRecord[]>([]);

const form = reactive({
  pluginId: "plugin.mediax",
  action: "switch" as "switch" | "rollback",
  fromVersion: "",
  toVersion: "",
  reason: "",
});

const loadAudits = async () => {
  if (!form.pluginId.trim()) {
    audits.value = [];
    return;
  }
  const result = await pluginOpsService.listAudits(form.pluginId, { page: 1, pageSize: 20 });
  audits.value = result.items;
};

const triggerAction = async () => {
  if (!form.pluginId.trim()) {
    return;
  }
  if (!canExecute.value) {
    return;
  }
  loading.value = true;
  try {
    await pluginOpsService.triggerAction(form.pluginId, {
      plugin_id: form.pluginId,
      from_version: form.fromVersion,
      to_version: form.toVersion,
      action: form.action,
      reason: form.reason,
    });
    await loadAudits();
  } finally {
    loading.value = false;
  }
};

onMounted(async () => {
  await loadUserContext();
  await loadAudits();
});
</script>
