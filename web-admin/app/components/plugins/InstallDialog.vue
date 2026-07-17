<template>
  <UModal
    v-model:open="open"
    :title="t('plugins.installDialog.title')"
    :description="plugin ? plugin.name || '' : t('plugins.installDialog.subtitle')"
    :ui="{ width: 'sm:max-w-lg' }"
  >
    <template #content>
      <div class="p-4 max-h-[80vh] overflow-y-auto">
        <div class="flex items-start gap-3">
          <img
            v-if="plugin?.icon"
            :src="plugin?.icon"
            alt=""
            class="w-10 h-10 rounded-md object-cover"
          />
          <div class="flex-1">
            <div class="font-medium text-[var(--text-primary)]">
              {{
                plugin
                  ? t("plugins.installDialog.headerTitleWithName", { name: plugin?.name || "-" })
                  : t("plugins.installDialog.headerTitle")
              }}
            </div>
            <div v-if="plugin" class="text-xs text-[var(--text-secondary)]">
              {{
                t("plugins.installDialog.meta", {
                  version: plugin?.version || "-",
                  author: plugin?.author || "-",
                })
              }}
            </div>
          </div>
        </div>

        <div class="mt-4">
          <UForm :state="state" class="space-y-4">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm mb-1 text-[var(--text-secondary)]"
                  >{{ t("plugins.installDialog.installMethod") }}</label
                >
                <USelect
                  v-model="state.installMode"
                  :items="installModeOptions"
                />
              </div>
              <div class="flex items-center gap-3 mt-6 md:mt-8">
                <USwitch v-model="state.enableAfterInstall" />
                <span class="text-sm text-[var(--text-secondary)]"
                  >{{ t("plugins.installDialog.enableAfterInstall") }}</span
                >
              </div>
            </div>

            <div
              class="rounded-md border border-[var(--border-color)] p-3 space-y-3"
            >
              <div class="text-sm font-medium text-[var(--text-primary)]">
                {{ t("plugins.installDialog.packageSource") }}
              </div>
              <UInput
                v-if="state.installMode === 'url'"
                v-model="state.url"
                placeholder="https://example.com/plugin.zip"
              >
                <template #leading>
                  <span class="inline-block shrink-0">
                    <UIcon name="i-heroicons-link" />
                  </span>
                </template>
              </UInput>
              <UInput
                v-if="state.installMode === 'url'"
                v-model="state.sha256"
                :placeholder="t('plugins.installDialog.sha256Placeholder')"
              />

              <div v-if="state.installMode === 'local'" class="space-y-2">
                <UButton icon="i-heroicons-folder-open" color="primary" variant="solid" @click="openLocalDirSelector">
                  {{ t("plugins.installDialog.chooseLocalSource") }}
                </UButton>
                <input
                  ref="localDirInputRef"
                  type="file"
                  webkitdirectory
                  directory
                  multiple
                  class="hidden"
                  @change="onLocalDirChange"
                />

                <div v-if="state.localDirName" class="text-xs text-[var(--text-secondary)]">
                  {{
                    t("plugins.installDialog.selectedDirectory", {
                      name: state.localDirName,
                      count: state.localDirFiles.length,
                    })
                  }}
                </div>
              </div>

              <div class="text-xs text-[var(--text-secondary)]">
                {{ t("plugins.installDialog.localUploadHelpPrefix") }}
                <code>plugin.yaml</code>
                {{ t("plugins.installDialog.localUploadHelpMiddle") }}
                <code>*.tar.gz/.tgz</code>
                {{ t("plugins.installDialog.localUploadHelpSuffix") }}
              </div>
            </div>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm mb-1 text-[var(--text-secondary)]"
                  >{{ t("plugins.installDialog.scope.label") }}</label
                >
                <USelect
                  v-model="state.scope"
                  :items="scopeOptions"
                  icon="i-heroicons-cog-6-tooth"
                />
              </div>
              <div>
                <label class="block text-sm mb-1 text-[var(--text-secondary)]"
                  >{{ t("plugins.installDialog.namespace.label") }}</label
                >
                <UInput
                  v-model="state.namespace"
                  :placeholder="t('plugins.installDialog.namespace.placeholder')"
                />
              </div>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm mb-1 text-[var(--text-secondary)]"
                  >{{ t("plugins.installDialog.environment") }}</label
                >
                <USelect
                  v-model="state.env"
                  :items="envOptions"
                  icon="i-heroicons-circle-stack"
                />
              </div>
              <div class="flex items-center gap-3 mt-6 md:mt-8">
                <USwitch v-model="state.autoUpdate" />
                <span class="text-sm text-[var(--text-secondary)]"
                  >{{ t("plugins.installDialog.autoUpdate") }}</span
                >
              </div>
            </div>
            <div class="flex items-center gap-3">
              <USwitch v-model="state.forceInstall" />
              <span class="text-sm text-[var(--text-secondary)]"
                >{{ t("plugins.installDialog.forceInstall") }}</span
              >
            </div>

            <div
              v-if="drainRequired"
              class="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-100"
            >
              <div class="font-medium text-amber-50">{{ t("plugins.installDialog.drain.title") }}</div>
              <div class="mt-1 text-amber-100/90">
                {{ drainNoticeText }}
              </div>
              <div class="mt-3 flex flex-wrap items-center gap-2">
                <UButton
                  v-if="!drainCreated"
                  size="sm"
                  color="warning"
                  variant="solid"
                  icon="i-heroicons-bolt"
                  :loading="creatingDrain"
                  :disabled="!drainTargetPluginId"
                  @click="createDrainFromInstall"
                >
                  {{ t("plugins.installDialog.drain.action") }}
                </UButton>
                <UButton
                  size="sm"
                  :color="drainCreated ? 'warning' : 'neutral'"
                  :variant="drainCreated ? 'solid' : 'ghost'"
                  icon="i-heroicons-arrow-right"
                  @click="goDrainDetail"
                >
                  {{
                    drainCreated
                      ? t("plugins.installDialog.drain.viewProgress")
                      : t("plugins.installDialog.drain.viewDetail")
                  }}
                </UButton>
              </div>
            </div>

            <div>
              <div class="text-sm mb-2 text-[var(--text-secondary)]">
                {{ t("plugins.installDialog.permissions.title") }}
              </div>
              <div
                class="rounded-md border border-[var(--border-color)] p-3 space-y-2"
              >
                <label class="flex items-center gap-2 text-sm">
                  <UCheckbox v-model="state.perms.network" />
                  <span>{{ t("plugins.installDialog.permissions.network") }}</span>
                </label>
                <label class="flex items-center gap-2 text-sm">
                  <UCheckbox v-model="state.perms.storage" />
                  <span>{{ t("plugins.installDialog.permissions.storage") }}</span>
                </label>
                <label class="flex items-center gap-2 text-sm">
                  <UCheckbox v-model="state.perms.files" />
                  <span>{{ t("plugins.installDialog.permissions.files") }}</span>
                </label>
              </div>
            </div>

            <div>
              <label class="block text-sm mb-1 text-[var(--text-secondary)]"
                >{{ t("plugins.installDialog.notes.label") }}</label
              >
              <UTextarea
                v-model="state.notes"
                :rows="3"
                :placeholder="t('plugins.installDialog.notes.placeholder')"
              />
            </div>
          </UForm>
        </div>

        <div class="mt-5 flex items-center justify-end gap-2">
          <UButton variant="ghost" @click="close">{{ t("plugins.installDialog.actions.cancel") }}</UButton>
          <UButton
            color="primary"
            :loading="installing"
            @click="confirmInstall"
            icon="i-heroicons-arrow-down-tray"
            >{{ t("plugins.installDialog.actions.install") }}</UButton
          >
        </div>
      </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import { storeToRefs } from "pinia";
import type { MarketplacePlugin } from "~/components/plugins/PluginCard.vue";

const router = useRouter();
const { t } = useI18n();

const props = defineProps<{
  modelValue: boolean;
  plugin?: MarketplacePlugin;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", v: boolean): void;
  (
    e: "installed",
    payload: { plugin: MarketplacePlugin | null; state: any }
  ): void;
}>();

const plugin = computed(() => props.plugin);

const open = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});

const userStore = useUserStore();
const { isRoot, isCurrentTenantAdmin } = storeToRefs(userStore);

type InstallMode = "url" | "local";

function resolveDefaultScope(): "system" | "org" | "user" {
  if (isRoot.value) return "system";
  if (isCurrentTenantAdmin.value) return "org";
  return "user";
}

const installModeOptions = computed(() => [
  { label: t("plugins.installDialog.mode.url"), value: "url" },
  { label: t("plugins.installDialog.mode.local"), value: "local" },
]);
const scopeOptions = computed(() => [
  { label: t("plugins.installDialog.scope.system"), value: "system" },
  { label: t("plugins.installDialog.scope.org"), value: "org" },
  { label: t("plugins.installDialog.scope.user"), value: "user" },
]);
const envOptions = [
  { label: "default", value: "default" },
  { label: "staging", value: "staging" },
  { label: "production", value: "production" },
];

const state = reactive({
  installMode: "url" as InstallMode,
  url: "",
  sha256: "",
  enableAfterInstall: true,
  localDirFiles: [] as File[],
  localDirName: "",
  scope: resolveDefaultScope(),
  namespace: "",
  env: envOptions[0].value,
  autoUpdate: true,
  forceInstall: true,
  perms: {
    network: true,
    storage: true,
    files: true,
  },
  notes: "",
});

const installing = ref(false);
const creatingDrain = ref(false);
const drainRequired = ref(false);
const drainCreated = ref(false);
const drainPluginId = ref("");
const localDirInputRef = ref<HTMLInputElement | null>(null);
const menuRefreshToken = useState<number>("px-menu-refresh-token", () => 0);

watch(
  () => props.modelValue,
  async (visible) => {
    if (!visible) return;
    if (!userStore.context) {
      try {
        await userStore.fetchUserContext();
      } catch {
        // ignore and fallback to current store snapshot
      }
    }
    state.scope = resolveDefaultScope();
  }
);

function close() {
  open.value = false;
}

const drainTargetPluginId = computed(() => drainPluginId.value || props.plugin?.id || "");
const drainNoticeText = computed(() =>
  drainCreated.value
    ? t("plugins.installDialog.drain.noticeCreated")
    : t("plugins.installDialog.drain.noticeRequired")
);

function goDrainDetail() {
  const id = drainTargetPluginId.value;
  if (!id) return;
  close();
  router.push(`/plugins/${encodeURIComponent(id)}`);
}

async function createDrainFromInstall() {
  const id = drainTargetPluginId.value;
  if (!id || creatingDrain.value) return;
  creatingDrain.value = true;
  const toast = useToast();
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    await svc.createDrainJob(id, {
      reason: "install requires plugin drain",
      mode: "drain",
    });
    toast.add({
      title: t("plugins.installDialog.toast.drainCreatedTitle"),
      description: t("plugins.installDialog.toast.drainCreatedDesc"),
      color: "success",
    });
    drainCreated.value = true;
  } catch (error: any) {
    console.error("create drain failed:", error);
    toast.add({
      title: t("plugins.installDialog.toast.drainCreateFailedTitle"),
      description: error?.message || t("plugins.installDialog.toast.drainCreateFailedDesc"),
      color: "error",
    });
  } finally {
    creatingDrain.value = false;
  }
}

function buildInstallMetadataPayload() {
  return {
    scope: state.scope || "system",
    namespace: (state.namespace || "").trim(),
    environment: state.env || "default",
    auto_update: Boolean(state.autoUpdate),
    permissions: {
      network: Boolean(state.perms.network),
      storage: Boolean(state.perms.storage),
      files: Boolean(state.perms.files),
    },
    notes: (state.notes || "").trim(),
  };
}

function onLocalDirChange(event: Event) {
  const toast = useToast();
  const target = event.target as HTMLInputElement;
  const files = Array.from(target.files || []);
  const hasPluginYAML = files.some((file) =>
    String((file as any)?.webkitRelativePath || file.name || "")
      .toLowerCase()
      .endsWith("/plugin.yaml")
  );
  const hasPackageArchive = files.some((file) => {
    const relPath = String((file as any)?.webkitRelativePath || file.name || "").toLowerCase();
    const name = relPath.split("/").pop() || "";
    return name.endsWith(".tar.gz") || name.endsWith(".tgz");
  });
  if (files.length > 0 && !hasPluginYAML && !hasPackageArchive) {
    state.localDirFiles = [];
    state.localDirName = "";
    target.value = "";
    toast.add({
      title: t("plugins.installDialog.toast.invalidDirectoryTitle"),
      description: t("plugins.installDialog.toast.invalidDirectoryDesc"),
      color: "red",
    });
    return;
  }
  state.localDirFiles = files;
  if (files.length > 0) {
    const rel = (files[0] as any)?.webkitRelativePath || "";
    state.localDirName = rel ? String(rel).split("/")[0] : t("plugins.installDialog.selectedDirectoryFallback");
  } else {
    state.localDirName = "";
  }
}

function openLocalDirSelector() {
  localDirInputRef.value?.click();
}

function notifyMenuRefresh() {
  menuRefreshToken.value += 1;
}

function isAlreadyInstalledConflict(error: any): boolean {
  const message = String(
    error?.message ||
      error?.data?.error ||
      error?.response?._data?.error ||
      error?.cause?.data?.error ||
      error?.cause?.response?._data?.error ||
      ""
  ).toLowerCase();
  const status = Number(error?.status || error?.statusCode || error?.cause?.status || 0);
  return (
    status === 409 &&
    (message.includes("already_exists") ||
      message.includes("already installed") ||
      message.includes("already_installed") ||
      message.includes("version already installed") ||
      message.includes("plugin version already installed"))
  );
}

function getErrorDetails(error: any): Record<string, any> {
  return (
    error?.details ||
    error?.data?.details ||
    error?.response?._data?.details ||
    error?.response?.data?.details ||
    error?.cause?.details ||
    {}
  );
}

function isDrainRequiredError(error: any): boolean {
  const details = getErrorDetails(error);
  const message = String(error?.message || error?.data?.error || error?.response?._data?.error || "").toLowerCase();
  return (
    details?.requires_drain === true ||
    details?.code === "PLUGIN_DRAIN_REQUIRED" ||
    message.includes("drain required") ||
    message.includes("plugin_drain_required")
  );
}

function resolveDrainPluginId(error: any): string {
  const details = getErrorDetails(error);
  return String(details?.plugin_id || props.plugin?.id || "").trim();
}

function isPlatformIncompatibleError(error: any): boolean {
  const details = getErrorDetails(error);
  return details?.code === "PLUGIN_PACKAGE_PLATFORM_INCOMPATIBLE";
}

function buildPlatformIncompatibleMessage(error: any): string {
  const details = getErrorDetails(error);
  const reason = String(details?.reason || "").trim();
  const path = String(details?.path || "").trim();
  const match = reason.match(/package target\s+([^\s]+)\s+\([^)]+\),\s+host target\s+([^\s]+)/i);
  if (match) {
    return t("plugins.installDialog.toast.platformIncompatibleTargets", {
      packageTarget: match[1],
      hostTarget: match[2],
    });
  }
  if (path) {
    return t("plugins.installDialog.toast.platformIncompatiblePath", { path });
  }
  return t("plugins.installDialog.toast.platformIncompatibleDesc");
}

async function confirmInstall() {
  drainRequired.value = false;
  drainCreated.value = false;
  drainPluginId.value = "";
  if (state.installMode === "url" && !state.url) {
    const toast = useToast();
    toast.add({
      title: t("common.error"),
      description: t("plugins.installDialog.toast.urlRequired"),
      color: "red",
    });
    return;
  }
  if (
    state.installMode === "local" &&
    state.localDirFiles.length === 0
  ) {
    const toast = useToast();
    toast.add({
      title: t("common.error"),
      description: t("plugins.installDialog.toast.localDirectoryRequired"),
      color: "red",
    });
    return;
  }

  installing.value = true;
  const toast = useToast();

  try {
    if (state.installMode === "url" && state.url) {
      const { useAdminPluginsService } = await import(
        "~/composables/api/services/adminPluginsService"
      );
      const svc = useAdminPluginsService();
      await svc.installFromUrl({
        url: state.url,
        sha256: state.sha256 || undefined,
        enable: !!state.enableAfterInstall,
        force: !!state.forceInstall,
        metadata: buildInstallMetadataPayload(),
      });

      toast.add({
        title: t("plugins.installDialog.toast.successTitle"),
        description: t("plugins.installDialog.toast.successDesc"),
        color: "green",
      });
    } else if (
      state.installMode === "local" &&
      state.localDirFiles.length > 0
    ) {
      const { useAdminPluginsService } = await import(
        "~/composables/api/services/adminPluginsService"
      );
      const svc = useAdminPluginsService();
      const formData = new FormData();
      for (const file of state.localDirFiles) {
        const relPath = (file as any)?.webkitRelativePath || file.name;
        formData.append("files", file, relPath);
        formData.append("file_paths", relPath);
      }
      formData.append("enable", String(!!state.enableAfterInstall));
      formData.append("force", String(!!state.forceInstall));
      formData.append("metadata", JSON.stringify(buildInstallMetadataPayload()));
      await svc.installFromLocal(formData);

      toast.add({
        title: t("plugins.installDialog.toast.successTitle"),
        description: t("plugins.installDialog.toast.successDesc"),
        color: "green",
      });
    } else {
      toast.add({
        title: t("plugins.installDialog.toast.noticeTitle"),
        description: t("plugins.installDialog.toast.sourceMissingDesc"),
        color: "warning",
      });
      return;
    }

    notifyMenuRefresh();
    emit("installed", {
      plugin: props.plugin || null,
      state: JSON.parse(JSON.stringify(state)),
    });
    close();
  } catch (error: any) {
    console.error("plugin install failed:", error);
    if (isAlreadyInstalledConflict(error)) {
      toast.add({
        title: t("plugins.installDialog.toast.alreadyInstalledTitle"),
        description: t("plugins.installDialog.toast.alreadyInstalledDesc"),
        color: "warning",
      });
      notifyMenuRefresh();
      return;
    }
    if (isDrainRequiredError(error)) {
      drainRequired.value = true;
      drainPluginId.value = resolveDrainPluginId(error);
      toast.add({
        title: t("plugins.installDialog.toast.drainRequiredTitle"),
        description: t("plugins.installDialog.toast.drainRequiredDesc"),
        color: "warning",
      });
      return;
    }
    if (isPlatformIncompatibleError(error)) {
      toast.add({
        title: t("plugins.installDialog.toast.platformIncompatibleTitle"),
        description: buildPlatformIncompatibleMessage(error),
        color: "warning",
      });
      return;
    }
    toast.add({
      title: t("plugins.installDialog.toast.installFailedTitle"),
      description: error?.message || t("plugins.installDialog.toast.installFailedDesc"),
      color: "error",
    });
  } finally {
    installing.value = false;
  }
}
</script>
