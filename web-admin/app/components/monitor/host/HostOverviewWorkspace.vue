<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useUserStore } from '~/stores/user';
import { useHostMonitorService, type HostSnapshot } from '~/composables/api/services/hostMonitorService';
import HostSectionNav, { type HostSection } from './HostSectionNav.vue';
import HostMetric from './HostMetric.vue';
import HostNetworkPanel from './HostNetworkPanel.vue';
import { isMainDisk, canAlertDisk, diskDetailReason } from './diskPresentation';
import { hostBytes, hostMessages, hostPercent, hostWarning } from './hostPresentation';

const props = withDefaults(defineProps<{ allowed?: boolean; identityKey?: string }>(), { allowed: undefined });
const user = useUserStore();
const { locale } = useI18n();
const text = computed(() => hostMessages[locale.value.toLowerCase().startsWith('zh') ? 'zh' : 'en']);
const allowed = computed(() => props.allowed ?? user.isRoot);
const identity = computed(() => `${props.identityKey ?? `${user.user?.id || ''}:${user.currentTenantUuid || ''}`}:${user.contextSessionStamp?.sessionId || ''}`);
const service = useHostMonitorService();
const section = ref<HostSection>('overview');
const showAllMounts = ref(false);
const snapshot = ref<HostSnapshot | null>(null);
const loading = ref(false);
const error = ref(false);
const stale = computed(() => error.value && !!snapshot.value);
const tabs = computed(() => [
  { value: 'overview' as const, label: text.value.overview, icon: 'i-heroicons-squares-2x2' },
  { value: 'compute' as const, label: text.value.compute, icon: 'i-lucide-cpu' },
  { value: 'disks' as const, label: text.value.disks, icon: 'i-lucide-hard-drive' },
  { value: 'network' as const, label: text.value.network, icon: 'i-lucide-network' },
  { value: 'gpu' as const, label: text.value.gpu, icon: 'i-lucide-monitor' },
]);
const bytes = (value?: number | null) => hostBytes(value, text.value.unavailable);
const percent = (value?: number | null) => hostPercent(value, text.value.unavailable);
const status = (value: string) => text.value[value as 'ok'] || text.value.unknown;
// df denominator excludes reserved blocks; do not silently use total_bytes.
function diskPercent(disk: HostSnapshot['disks'][number]) {
  if (disk.used_bytes == null || disk.available_bytes == null || disk.used_bytes + disk.available_bytes <= 0) return null;
  return disk.used_bytes / (disk.used_bytes + disk.available_bytes) * 100;
}
const persistentDisks = computed(() => snapshot.value?.disks.filter(isMainDisk) || []);
const secondaryDisks = computed(() => snapshot.value?.disks.filter(disk => !isMainDisk(disk)) || []);
const displayedDisks = computed(() => showAllMounts.value ? snapshot.value?.disks || [] : persistentDisks.value);
const mountNote = (disk: HostSnapshot['disks'][number]) => {
  const reason = diskDetailReason(disk);
  const zh = locale.value.startsWith('zh');
  if (reason === 'image') return zh ? '系统 / 模拟器镜像 · 不参与容量告警' : 'System / simulator image · excluded from capacity alerts';
  if (reason === 'internal') return zh ? '系统内部挂载 · 不参与容量告警' : 'Internal mount · excluded from capacity alerts';
  if (disk.read_only === true) return zh ? '只读 · 不参与容量告警' : 'Read-only · excluded from capacity alerts';
  return '';
};
const primaryDisk = computed(() => persistentDisks.value.find(disk => disk.mount_point === '/') || [...persistentDisks.value].sort((a, b) => (diskPercent(b) ?? -1) - (diskPercent(a) ?? -1))[0]);
const pressuredDisks = computed(() => persistentDisks.value.filter(disk => canAlertDisk(disk) && (diskPercent(disk) ?? 0) >= 90));
const diskWarning = computed(() => `${locale.value.startsWith('zh') ? '磁盘可用空间不足' : 'Low disk space'}: ${pressuredDisks.value.map(disk => `${disk.mount_point} ${percent(diskPercent(disk))}`).join(' · ')}`);
const uptime = computed(() => {
  const seconds = snapshot.value?.uptime_seconds;
  if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return text.value.unavailable;
  return `${Math.floor(seconds / 86400)} ${text.value.days} ${Math.floor(seconds % 86400 / 3600)} ${text.value.hours}`;
});
const sampled = computed(() => {
  if (!snapshot.value) return text.value.never;
  return new Date(snapshot.value.collected_at).toLocaleString(locale.value.startsWith('zh') ? 'zh-CN' : 'en-US');
});
let controller: AbortController | undefined;
let generation = 0;
let mounted = false;
async function refresh() {
  if (!allowed.value || loading.value) return;
  const current = ++generation;
  controller = new AbortController();
  loading.value = true;
  try {
    const result = await service.getSnapshot(controller.signal);
    if (current !== generation || !allowed.value) return;
    snapshot.value = result;
    error.value = false;
  } catch {
    if (current === generation && allowed.value) error.value = true;
  } finally {
    if (current === generation) loading.value = false;
  }
}
function reset() {
  ++generation;
  controller?.abort();
  snapshot.value = null;
  error.value = false;
  loading.value = false;
  section.value = 'overview';
  showAllMounts.value = false;
}
watch([allowed, identity], () => { reset(); if (mounted && allowed.value) void refresh(); });
onMounted(() => { mounted = true; void refresh(); });
onBeforeUnmount(() => { mounted = false; reset(); });
</script>

<template>
  <section class="mx-auto min-w-0 max-w-7xl space-y-4 p-4 sm:p-6" data-testid="host-overview">
    <header class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0"><h1 class="text-2xl font-semibold">{{ text.title }}</h1><p class="mt-1 text-sm text-muted">{{ text.manual }}</p></div>
      <UButton v-if="allowed" data-testid="host-refresh" icon="i-lucide-refresh-cw" color="neutral" variant="outline" :loading="loading" :disabled="loading" @click="refresh">{{ text.refresh }}</UButton>
    </header>
    <UAlert variant="soft" v-if="!allowed" color="warning" :title="text.rootOnly" />
    <template v-else>
      <div class="flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted" aria-live="polite">
        <span>{{ text.sampled }}: <time v-if="snapshot" :datetime="snapshot.collected_at">{{ sampled }}</time><span v-else>{{ text.never }}</span></span>
        <span v-if="snapshot">{{ snapshot.hostname || text.unavailable }} · {{ snapshot.os }} / {{ snapshot.arch }}</span>
        <span v-if="loading" role="status">{{ text.loading }}</span>
      </div>
      <UAlert variant="soft" v-if="error" data-testid="host-error" color="error" :title="text.failed" />
      <UAlert variant="soft" v-if="stale" data-testid="host-stale" color="warning" :title="text.stale" />
      <UAlert variant="soft" v-if="pressuredDisks.length" data-testid="host-disk-warning" color="warning" :title="diskWarning" />
      <UAlert variant="soft" v-if="snapshot?.warnings.length" color="neutral" :title="text.warnings" :description="[...new Set(snapshot.warnings.map(message => hostWarning(message, text)))].join(' · ')" class="break-words" />
      <HostSectionNav v-model="section" :items="tabs" :label="text.title">
        <div v-if="!snapshot" class="rounded-lg border border-default p-6 text-muted" role="status">{{ loading ? text.loading : text.empty }}</div>
        <template v-else>
          <div v-if="section === 'overview'" class="space-y-4">
            <div class="grid min-w-0 gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <HostMetric :label="text.cpu" :value="percent(snapshot.cpu.usage_percent)" :detail="`${snapshot.cpu.logical_cores} ${text.cores} · ${status(snapshot.cpu.status)}`" />
              <HostMetric :label="text.memory" :value="bytes(snapshot.memory.used_bytes)" :detail="`${text.total}: ${bytes(snapshot.memory.total_bytes)} · ${status(snapshot.memory.status)}`" />
              <HostMetric :label="`${text.disks} ${primaryDisk?.mount_point || ''}`" :value="percent(primaryDisk ? diskPercent(primaryDisk) : null)" :detail="`${text.available}: ${bytes(primaryDisk?.available_bytes)}`" />
              <HostMetric :label="text.uptime" :value="uptime" />
            </div>
            <UCard>
              <dl class="grid gap-4 text-sm sm:grid-cols-2">
                <div><dt class="text-muted">{{ text.scope }}</dt><dd class="break-words">{{ text[snapshot.scope] || text.unknown }} · {{ text.scopeNote }}</dd></div>
                <div><dt class="text-muted">{{ text.container }}</dt><dd>{{ snapshot.container_detected ? text.yes : text.no }}</dd></div>
                <div><dt class="text-muted">{{ text.network }}</dt><dd>{{ status(snapshot.network.status) }} · {{ snapshot.network.interfaces.length }} {{ text.interface }}</dd></div>
                <div><dt class="text-muted">GPU</dt><dd>{{ status(snapshot.gpu.status) }} · {{ snapshot.gpu.devices.length }} {{ text.devices }}</dd></div>
              </dl>
            </UCard>
          </div>
          <div v-else-if="section === 'compute'" class="space-y-4">
            <h2 class="text-lg font-semibold">{{ text.compute }}</h2>
            <div class="grid gap-3 sm:grid-cols-2">
              <HostMetric :label="text.cpu" :value="percent(snapshot.cpu.usage_percent)" :detail="status(snapshot.cpu.status)" />
              <HostMetric :label="text.cores" :value="String(snapshot.cpu.logical_cores)" :detail="`${text.sample}: ${snapshot.cpu.sample_ms} ms`" />
              <HostMetric v-for="key in (['memory', 'swap'] as const)" :key="key" :label="text[key]" :value="bytes(snapshot[key].used_bytes)" :detail="`${status(snapshot[key].status)} · ${text.available}: ${bytes(snapshot[key].available_bytes)} / ${text.total}: ${bytes(snapshot[key].total_bytes)}`" />
            </div>
            <p class="text-xs text-muted">{{ text.duration }}: {{ snapshot.duration_ms }} ms</p>
          </div>
          <div v-else-if="section === 'disks'" class="space-y-3">
            <div class="flex flex-wrap items-center justify-between gap-2"><h2 class="text-lg font-semibold">{{ text.disks }}</h2><UButton to="/monitor/storage" color="neutral" variant="outline" icon="i-lucide-hard-drive">{{ text.storageGovernance }}</UButton></div>
            <p class="text-sm text-muted">{{ locale.startsWith('zh') ? '占用率 = 已用 / (已用 + 可用)，与 df 口径一致；保留空间 = 总量 − 已用 − 可用。' : 'Usage = used / (used + available), matching df. Reserved space = total − used − available.' }}</p>
            <p class="text-sm text-muted">{{ locale.startsWith('zh') ? '默认展示系统盘、数据盘和外接盘；镜像及系统内部挂载收在全部挂载中。' : 'System, data and external disks are shown by default. Images and internal mounts are in all mounts.' }}</p>
            <UButton v-if="secondaryDisks.length" color="neutral" variant="outline" data-testid="host-all-mounts" :aria-expanded="showAllMounts" @click="showAllMounts = !showAllMounts">{{ showAllMounts ? (locale.startsWith('zh') ? '收起内部挂载' : 'Hide internal mounts') : (locale.startsWith('zh') ? '查看全部挂载' : 'Show all mounts') }} ({{ secondaryDisks.length }})</UButton>
            <p v-if="!displayedDisks.length" class="text-muted">{{ text.noDisks }}</p>
            <div v-else class="min-w-0 overflow-x-auto rounded-lg border border-default">
              <table class="w-full text-left text-sm"><thead><tr><th>{{ text.mount }}</th><th>{{ text.filesystem }}</th><th>{{ text.used }}</th><th>{{ text.available }}</th><th>{{ text.total }}</th><th>%</th></tr></thead>
                <tbody><tr v-for="disk in displayedDisks" :key="`${disk.mount_point}:${disk.filesystem}`"><td class="break-all">{{ disk.mount_point }}<span v-if="mountNote(disk)" class="mt-1 block text-xs text-muted">{{ mountNote(disk) }}</span><span v-if="disk.status !== 'ok'" class="block text-warning">{{ status(disk.status) }}</span></td><td>{{ disk.filesystem }}</td><td>{{ bytes(disk.used_bytes) }}</td><td>{{ bytes(disk.available_bytes) }}</td><td>{{ bytes(disk.total_bytes) }}</td><td>{{ percent(diskPercent(disk)) }}</td></tr></tbody>
              </table>
            </div>
          </div>
          <HostNetworkPanel v-else-if="section === 'network'" :network="snapshot.network" />
          <div v-else class="space-y-3">
            <h2 class="text-lg font-semibold">GPU · {{ status(snapshot.gpu.status) }}</h2>
            <UAlert variant="soft" v-if="snapshot.gpu.status === 'unsupported'" color="neutral" :title="text.gpuUnsupported" />
            <p v-else-if="!snapshot.gpu.devices.length" class="text-muted">{{ text.noDevices }}</p>
            <UCard v-for="device in snapshot.gpu.devices" :key="device.index">
              <h3 class="break-words font-semibold">{{ device.index }} · {{ device.name }}</h3>
              <dl class="mt-3 grid gap-3 text-sm sm:grid-cols-3"><div><dt class="text-muted">{{ text.utilization }}</dt><dd>{{ percent(device.utilization_percent) }}</dd></div><div><dt class="text-muted">{{ text.memory }}</dt><dd>{{ bytes(device.memory_used_bytes) }} / {{ bytes(device.memory_total_bytes) }}</dd></div><div><dt class="text-muted">{{ text.temperature }}</dt><dd>{{ device.temperature_celsius == null ? text.unavailable : `${device.temperature_celsius} °C` }}</dd></div></dl>
            </UCard>
          </div>
        </template>
      </HostSectionNav>
    </template>
  </section>
</template>

<style scoped>
th, td { padding: 0.75rem; border-bottom: 1px solid var(--ui-border); }
th { white-space: nowrap; }
</style>
