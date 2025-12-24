<template>
  <div class="p-6 space-y-6">
    <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ $t("settings.openCapabilities.title") }}
        </h1>
        <p class="text-gray-600 dark:text-gray-400">
          {{ $t("settings.openCapabilities.description") }}
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <UButton
          icon="i-heroicons-arrow-path"
          :loading="loading"
          @click="refresh"
        >
          {{ $t("settings.openCapabilities.actions.refresh") }}
        </UButton>
        <UButton
          variant="ghost"
          icon="i-heroicons-sparkles"
          @click="refresh"
        >
          {{ $t("settings.openCapabilities.actions.syncNow") }}
        </UButton>
      </div>
    </div>

    <UAlert
      v-if="!allowAccess"
      icon="i-heroicons-lock-closed"
      color="amber"
      variant="subtle"
      :title="$t('settings.openCapabilities.noAccess.title')"
      :description="$t('settings.openCapabilities.noAccess.description')"
    />

    <div v-else class="space-y-4">
      <UCard>
        <template #header>
          <div class="flex flex-wrap items-center gap-4 text-sm text-gray-600 dark:text-gray-400">
            <div class="flex items-center gap-2">
              <UIcon name="i-heroicons-rectangle-group" class="h-4 w-4" />
              <span>
                {{
                  $t('settings.openCapabilities.summary.modules', {
                    count: stats.totalModules,
                  })
                }}
              </span>
            </div>
            <div class="flex items-center gap-2">
              <UIcon name="i-heroicons-cube" class="h-4 w-4" />
              <span>
                {{
                  $t('settings.openCapabilities.summary.capabilities', {
                    count: stats.totalCapabilities,
                  })
                }}
              </span>
            </div>
            <div class="flex items-center gap-2">
              <UIcon name="i-heroicons-clock" class="h-4 w-4" />
              <span>
                {{
                  $t('settings.openCapabilities.summary.generatedAt', {
                    time: formatDate(generatedAt),
                  })
                }}
              </span>
            </div>
            <div class="flex items-center gap-2">
              <UIcon name="i-heroicons-arrow-path" class="h-4 w-4" />
              <span>
                {{
                  $t('settings.openCapabilities.summary.lastLoadedAt', {
                    time: formatDate(lastLoadedAt),
                  })
                }}
              </span>
            </div>
          </div>
        </template>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
          <USelect
            v-model="filters.module"
            :items="moduleOptions"
            option-attribute="label"
            value-attribute="value"
            icon="i-heroicons-squares-2x2"
          />
          <USelect
            v-model="filters.protocol"
            :items="protocolOptions"
            option-attribute="label"
            value-attribute="value"
            icon="i-heroicons-arrows-up-down"
          />
          <UInput
            v-model="filters.search"
            icon="i-heroicons-magnifying-glass"
            :placeholder="$t('settings.openCapabilities.filters.searchPlaceholder')"
          />
        </div>
        <div class="flex gap-2 pt-3">
          <UButton size="sm" variant="ghost" @click="resetFilters">
            {{ $t('settings.openCapabilities.filters.reset') }}
          </UButton>
        </div>
      </UCard>

      <UAlert
        v-if="!loading && renderedModules.length === 0"
        icon="i-heroicons-information-circle"
        variant="subtle"
        class="border-dashed"
        :title="$t('settings.openCapabilities.empty.title')"
        :description="$t('settings.openCapabilities.empty.description')"
      />

      <UCard v-if="loading">
        <div class="flex items-center gap-2 text-gray-500">
          <UIcon name="i-heroicons-arrow-path" class="h-4 w-4 animate-spin" />
          <span>{{ $t('settings.openCapabilities.loading') }}</span>
        </div>
      </UCard>

      <div v-for="group in renderedModules" :key="group.module.module" class="space-y-3">
        <UCard>
          <template #header>
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <div class="flex items-center gap-2">
                  <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                    {{ formatModuleName(group.module) }}
                  </h2>
                  <UBadge variant="soft" color="gray">
                    {{ group.module.capabilityCount }}
                  </UBadge>
                </div>
                <p class="text-sm text-gray-600 dark:text-gray-400">
                  {{ group.module.description || $t('settings.openCapabilities.defaultModuleDescription') }}
                </p>
              </div>
              <div class="flex flex-wrap gap-2">
                <UBadge
                  v-for="channel in group.module.protocolChannels"
                  :key="channel"
                  size="xs"
                  variant="subtle"
                  :color="protocolColor(channel)"
                >
                  {{ formatProtocol(channel) }}
                </UBadge>
              </div>
            </div>
          </template>

          <div v-if="group.capabilities.length === 0" class="text-sm text-gray-500 py-8 text-center">
            {{ $t('settings.openCapabilities.empty.filtered') }}
          </div>

          <div v-else class="space-y-4">
            <div
              v-for="capability in group.capabilities"
              :key="capability.capabilityId"
              class="rounded-lg border border-gray-200 dark:border-gray-800 p-4 space-y-3"
            >
              <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                <div>
                  <div class="flex items-center gap-2">
                    <h3 class="text-base font-semibold text-gray-900 dark:text-white">
                      {{ capability.title || capability.capabilityId }}
                    </h3>
                    <UBadge size="xs" :color="statusColor(capabilityStatus(capability))" variant="subtle">
                      {{ $t(`settings.openCapabilities.status.${capabilityStatus(capability)}`) }}
                    </UBadge>
                    <UBadge v-if="capability.preferredProtocol" size="xs" variant="outline">
                      {{ $t('settings.openCapabilities.labels.preferred') }}:
                      {{ formatProtocol(capability.preferredProtocol) }}
                    </UBadge>
                  </div>
                  <p class="font-mono text-xs text-gray-500">
                    {{ capability.capabilityId }}
                  </p>
                  <p v-if="capability.description" class="text-sm text-gray-600 dark:text-gray-300">
                    {{ capability.description }}
                  </p>
                </div>
                <div class="flex flex-col gap-2 text-sm text-right min-w-[200px]">
                  <div class="text-xs text-gray-500">
                    {{ $t('settings.openCapabilities.labels.hash') }}:
                    <span class="font-mono">{{ truncateHash(capability.capabilitiesHash) }}</span>
                  </div>
                  <div class="text-xs text-gray-500">
                    {{ $t('settings.openCapabilities.labels.plugin') }}:
                    <span>{{ capability.pluginId || '-' }} · {{ capability.pluginVersion || '-' }}</span>
                  </div>
                </div>
              </div>

              <div class="flex flex-wrap gap-2">
                <UBadge
                  v-for="binding in capability.protocols"
                  :key="`${capability.capabilityId}-${binding.channel}-${binding.endpoint}`"
                  variant="outline"
                  size="xs"
                  :color="protocolColor(binding.channel)"
                >
                  {{ formatProtocol(binding.channel) }}
                  <template v-if="binding.toolRef">
                    · {{ binding.toolRef }}
                  </template>
                  <template v-else-if="binding.method">
                    · {{ binding.method }}
                  </template>
                  <template v-else-if="binding.rpc">
                    · {{ binding.rpc }}
                  </template>
                </UBadge>
              </div>

              <div
                v-if="capability.debugExamples.tenantInvocationPayload"
                class="bg-gray-900/90 text-gray-100 rounded-lg p-3 text-xs overflow-auto"
              >
                <p class="mb-1 font-semibold">
                  {{ $t('settings.openCapabilities.labels.payloadSample') }}
                </p>
                <pre class="whitespace-pre-wrap">
{{ formatPayload(capability.debugExamples.tenantInvocationPayload) }}
                </pre>
              </div>

              <div class="flex flex-wrap gap-2">
                <UButton
                  size="xs"
                  icon="i-heroicons-clipboard"
                  @click="copyCurl(capability)"
                >
                  {{ $t('settings.openCapabilities.actions.copyCurl') }}
                </UButton>
                <UButton
                  size="xs"
                  variant="soft"
                  icon="i-heroicons-arrow-down-on-square"
                  @click="copyInsomnia(capability)"
                >
                  {{ $t('settings.openCapabilities.actions.copyInsomnia') }}
                </UButton>
                <UButton
                  v-for="doc in capability.docs"
                  :key="doc"
                  size="xs"
                  variant="ghost"
                  icon="i-heroicons-arrow-top-right-on-square"
                  :to="doc"
                  target="_blank"
                >
                  {{ $t('settings.openCapabilities.actions.openDoc') }}
                </UButton>
                <UButton
                  v-if="capability.module === 'media'"
                  size="xs"
                  color="purple"
                  variant="soft"
                  icon="i-heroicons-link"
                  @click="openMediaEntry(capability)"
                >
                  {{ $t('settings.openCapabilities.actions.openMediaEntry') }}
                </UButton>
              </div>
            </div>
          </div>
        </UCard>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { storeToRefs } from "pinia";
import { onMounted, computed, reactive, ref, watch } from "vue";
import {
  PlatformCapabilityService,
  type PlatformCapabilityModule,
  type PlatformCapability,
} from "~/composables/api/services/platformCapabilityService";
import { useUserStore } from "~/stores/user";
import { useCopy } from "~/composables/useCopy";

definePageMeta({
  title: "开放能力",
  icon: "i-heroicons-bolt",
  order: 15,
});

const { t } = useI18n({ useScope: "global" });
const toast = useToast();
const userStore = useUserStore();
const { isRoot } = storeToRefs(userStore);
const allowAccess = computed(() => isRoot.value);

type ModuleView = {
  module: PlatformCapabilityModule;
  capabilities: PlatformCapability[];
};

const modules = ref<PlatformCapabilityModule[]>([]);
const generatedAt = ref<string>();
const lastLoadedAt = ref<string>();
const loading = ref(false);
const filters = reactive({
  module: "all",
  protocol: "all",
  search: "",
});

const stats = computed(() => ({
  totalModules: modules.value.length,
  totalCapabilities: modules.value.reduce(
    (sum, mod) => sum + (mod.capabilityCount ?? mod.capabilities.length ?? 0),
    0
  ),
}));

const moduleOptions = computed(() => {
  const seen = new Set<string>();
  const options = modules.value
    .filter((mod) => {
      if (seen.has(mod.module)) return false;
      seen.add(mod.module);
      return true;
    })
    .map((mod) => ({
      label: mod.displayName || mod.module,
      value: mod.module,
    }));
  return [
    {
      label: t("settings.openCapabilities.filters.moduleAll"),
      value: "all",
    },
    ...options,
  ];
});

const protocolOptions = computed(() => [
  {
    label: t("settings.openCapabilities.filters.protocolAll"),
    value: "all",
  },
  { label: "MCP", value: "mcp" },
  { label: "REST", value: "rest" },
  { label: "gRPC", value: "grpc" },
  { label: "Workflow", value: "workflow" },
  { label: "Composite", value: "composite" },
]);

const renderedModules = computed<ModuleView[]>(() => {
  const searchTerm = filters.search.trim().toLowerCase();
  const moduleFilter = filters.module;
  const protocolFilter = filters.protocol;
  const filterActive = Boolean(searchTerm || protocolFilter !== "all");

  return modules.value
    .filter((mod) => moduleFilter === "all" || mod.module === moduleFilter)
    .map((mod) => {
      const baseCapabilities = mod.capabilities || [];
      if (!filterActive) {
        return { module: mod, capabilities: baseCapabilities };
      }
      const filteredCaps = baseCapabilities.filter((cap) => {
        const matchesProtocol =
          protocolFilter === "all" ||
          cap.protocols.some(
            (binding) => binding.channel?.toLowerCase() === protocolFilter
          );
        if (!matchesProtocol) return false;
        if (!searchTerm) return true;
        const pool = [
          cap.capabilityId,
          cap.title,
          cap.description,
          cap.module,
        ]
          .filter(Boolean)
          .map((val) => (val as string).toLowerCase());
        return pool.some((txt) => txt.includes(searchTerm));
      });
      return { module: mod, capabilities: filteredCaps };
    })
    .filter((group) => group.capabilities.length > 0 || !filterActive);
});

const { copy } = useCopy();

const refresh = async () => {
  if (!allowAccess.value) return;
  loading.value = true;
  try {
    const result = await PlatformCapabilityService.listModules();
    modules.value = result.modules;
    generatedAt.value = result.generatedAt;
    lastLoadedAt.value = new Date().toISOString();
  } catch (error: any) {
    console.error("加载平台能力失败", error);
    toast.add({
      title: t("settings.openCapabilities.toast.loadFailed"),
      description: error?.message || String(error),
      color: "red",
    });
  } finally {
    loading.value = false;
  }
};

const resetFilters = () => {
  filters.module = "all";
  filters.protocol = "all";
  filters.search = "";
};

onMounted(async () => {
  try {
    await userStore.fetchUserContext();
  } catch (error) {
    console.warn("加载用户上下文失败", error);
  }
  if (allowAccess.value) {
    await refresh();
  }
});

watch(
  () => allowAccess.value,
  async (allowed) => {
    if (allowed && modules.value.length === 0) {
      await refresh();
    }
  }
);

const formatModuleName = (mod: PlatformCapabilityModule) =>
  mod.displayName || mod.module;

const formatProtocol = (channel?: string) =>
  channel ? channel.toUpperCase() : "-";

const formatPayload = (payload: Record<string, any>) =>
  JSON.stringify(payload, null, 2);

const formatDate = (value?: string) => {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(Number(date))) return value;
  return date.toLocaleString();
};

const protocolColor = (channel?: string) => {
  switch ((channel || '').toLowerCase()) {
    case 'mcp':
      return 'purple';
    case 'grpc':
      return 'blue';
    case 'rest':
      return 'green';
    case 'workflow':
      return 'indigo';
    case 'composite':
      return 'orange';
    default:
      return 'gray';
  }
};

const capabilityStatus = (capability: PlatformCapability) => {
  const states = capability.protocols.map((binding) =>
    (binding.healthState || 'healthy').toLowerCase()
  );
  if (states.every((state) => state === 'offline')) return 'offline';
  if (states.some((state) => state === 'degraded')) return 'degraded';
  return 'active';
};

const statusColor = (status: string) => {
  switch (status) {
    case 'active':
      return 'green';
    case 'degraded':
      return 'amber';
    case 'offline':
      return 'gray';
    default:
      return 'gray';
  }
};

const truncateHash = (value?: string) => {
  if (!value) return '-';
  if (value.length <= 12) return value;
  return `${value.slice(0, 6)}…${value.slice(-4)}`;
};

const buildCurlSnippet = (capability: PlatformCapability) => {
  if (capability.debugExamples.tenantInvocationCurl) {
    return capability.debugExamples.tenantInvocationCurl;
  }
  const payload = capability.debugExamples.tenantInvocationPayload || {};
  const preferred =
    capability.preferredProtocol || capability.protocols[0]?.channel || 'rest';
  const body = JSON.stringify(
    {
      capability_id: capability.capabilityId,
      preferred_protocol: preferred,
      idempotency_key: 'demo-request-id',
      payload,
    },
    null,
    2
  );
  return `curl -X POST "$POWERX_BASE_URL/tenant/invocations" \\
  -H "Authorization: Bearer $TENANT_TOKEN" \\
  -H "X-PowerX-Tenant: $TENANT_UUID" \\
  -H "Content-Type: application/json" \\
  -d '${body}'`;
};

const buildInsomniaSnippet = (capability: PlatformCapability) => {
  const preferred =
    capability.preferredProtocol || capability.protocols[0]?.channel || 'rest';
  const payload = capability.debugExamples.tenantInvocationPayload || {};
  const body = JSON.stringify(
    {
      capability_id: capability.capabilityId,
      preferred_protocol: preferred,
      idempotency_key: 'demo-request-id',
      payload,
    },
    null,
    2
  );
  return JSON.stringify(
    {
      _type: 'request',
      name: capability.title || capability.capabilityId,
      method: 'POST',
      url: '{{ POWERX_BASE_URL }}/tenant/invocations',
      headers: [
        { name: 'Authorization', value: 'Bearer {{ TENANT_TOKEN }}' },
        { name: 'X-PowerX-Tenant', value: '{{ TENANT_UUID }}' },
        { name: 'Content-Type', value: 'application/json' },
      ],
      body: {
        mimeType: 'application/json',
        text: body,
      },
    },
    null,
    2
  );
};

const copyCurl = async (capability: PlatformCapability) => {
  const ok = await copy(buildCurlSnippet(capability), {
    successText: t('settings.openCapabilities.toast.copySuccess'),
    failText: t('settings.openCapabilities.toast.copyFailed'),
  });
  return ok;
};

const copyInsomnia = async (capability: PlatformCapability) => {
  const ok = await copy(buildInsomniaSnippet(capability), {
    successText: t('settings.openCapabilities.toast.copySuccess'),
    failText: t('settings.openCapabilities.toast.copyFailed'),
  });
  return ok;
};

const openMediaEntry = (capability: PlatformCapability) => {
  if (process.server) return;
  const restEndpoint = capability.protocols.find(
    (binding) => binding.channel?.toLowerCase() === 'rest' && binding.endpoint
  )?.endpoint;
  const target = restEndpoint || '/media/assets';
  window.open(target, '_blank');
};
</script>
