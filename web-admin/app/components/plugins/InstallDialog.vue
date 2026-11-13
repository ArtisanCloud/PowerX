<template>
  <UModal
    v-model:open="open"
    :title="t('plugins.installDialog.title')"
    :description="plugin ? plugin.name || '' : t('plugins.installDialog.subtitle')"
    :ui="{ content: 'w-[calc(100vw-2rem)] sm:max-w-lg' }"
    :dismissible="true"
    :close="true"
  >
    <template #body>
      <div class="max-h-[70vh] overflow-y-auto">
        <div v-if="plugin && !recovery && !showTask" class="flex items-start gap-3">
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

        <PluginInstallTaskProgress v-if="showTask && currentSession.task" :task="currentSession.task" :connection="currentSession.connection" :busy="installing || checkingRecovery || currentSession.cancellingUpload" :uploading="installing && currentSession.task.status === 'awaiting_upload'" :cancelling="currentSession.cancellingUpload" :error="recoveryError"
          @check="checkTask()" @upload="currentSession.taskVisible = false" @discard="discardTask" @retry="reviewTask" @recover="recoverTask" @done="completeTask" @close="close" />
        <details v-if="!showTask && !recovery && recentTasks.length" class="my-3 rounded-lg border border-[var(--border-color)] p-3 text-sm">
          <summary>{{ t('plugins.installTask.recent') }}</summary>
          <div class="mt-2 grid gap-2"><button v-for="item in recentTasks.slice(0, 8)" :key="item.task_id" type="button" class="rounded-lg border border-[var(--border-color)] p-2 text-left hover:bg-[var(--bg-tertiary)]" @click="openTask(item)">
            <span class="block break-all">{{ item.plugin_id || '—' }} · {{ item.version || '—' }}</span><span class="text-xs text-[var(--text-secondary)]">{{ t(`plugins.installTask.status.${item.status}`) }}</span>
          </button></div>
        </details>
        <PluginInstallRecovery v-if="recovery" :state="recovery" :checking="checkingRecovery" :busy="installing" :error="recoveryError" @check="checkInstallation" @resume="resumeInstallation" @done="completeRecovery" @view="viewInstalled" />
        <UAlert v-if="currentSession.creationUncertain && !currentSession.task" class="mt-4" color="warning" variant="soft" :title="t('plugins.installTask.creationUnknown')" :description="t('plugins.installTask.creationUnknownDescription')" />
        <div v-show="!recovery && !showTask && !currentSession.creationUncertain" class="mt-4">
          <p v-if="recoveryError" role="alert" class="mb-3 break-words text-sm text-red-500">{{ recoveryError }}</p>
          <UForm :key="currentSession.key" :state="state" class="space-y-4">
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
                  >{{ t("plugins.installDialog.releaseChannel") }}</label
                >
                <USelect
                  v-model="state.releaseChannel"
                  :items="releaseChannelOptions"
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
            <UAlert v-if="replacementPreview" color="warning" variant="soft" :title="t('plugins.installFlow.currentInstallation')" :description="`${replacementPreview.current.version} → ${replacementPreview.target.version} · ${replacementPreview.target.provider.id} · ${replacementPreview.target.provider.authority}`" />
            <UCheckbox v-if="replacementPreview?.requires_provider_confirmation" v-model="providerBindingConfirmed" :label="t('plugins.installFlow.providerBindingConfirmation')" />
            <div class="flex items-center gap-3">
              <USwitch v-model="state.forceInstall" :disabled="state.installMode !== 'local' && !replacementPreview" />
              <span class="text-sm text-[var(--text-secondary)]"
                >{{ t("plugins.installDialog.forceInstall") }}</span
              >
            </div>

            <p v-if="state.installMode !== 'local' && !replacementPreview" class="text-xs text-[var(--text-secondary)]">{{ t('plugins.installFlow.localPreviewHelp') }}</p>
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

      </div>
    </template>
    <template v-if="!recovery && !showTask" #footer>
        <div class="flex w-full items-center justify-end gap-2">
          <UButton variant="ghost" :disabled="installing" @click="close">{{ t("plugins.installDialog.actions.cancel") }}</UButton>
          <UButton
            color="primary"
            :loading="installing"
            :disabled="!!replacementPreview && (!state.forceInstall || (replacementPreview.requires_provider_confirmation && !providerBindingConfirmed))"
            @click="confirmInstall"
            icon="i-heroicons-arrow-down-tray"
            >{{ currentSession.creationUncertain ? t("plugins.installTask.continueRequest") : t("plugins.installDialog.actions.install") }}</UButton
          >
        </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import { storeToRefs } from "pinia";
import { useAdminPluginsService } from '~/composables/api/services/adminPluginsService';
import { shallowRef } from "vue";
import PluginInstallTaskProgress from '~/components/plugins/PluginInstallTaskProgress.vue';
import { acceptInstallTask, installTaskTerminal, type PluginInstallTask } from '~/utils/plugin-install-task';
import { connectBackupEvents } from '~/utils/backup-sse';
import { readAuthSession, AUTH_SESSION_EVENT } from '~/utils/auth-session';
import type { MarketplacePlugin } from "~/components/plugins/PluginCard.vue";
import PluginInstallRecovery from '~/components/plugins/PluginInstallRecovery.vue';
import { canResumeInstall, installPackageIdentity, readLocalInstallIdentity, reconcileInstall, type InstallRecovery } from '~/utils/plugin-install-recovery';

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
const releaseChannelOptions = [
  { label: "stable", value: "stable" },
  { label: "beta", value: "beta" },
  { label: "canary", value: "canary" },
];

function createInstallForm() {
  return {
    installMode: "url" as InstallMode,
    url: "",
    sha256: "",
    enableAfterInstall: true,
    localDirFiles: [] as File[],
    localDirName: "",
    scope: resolveDefaultScope(),
    namespace: "",
    releaseChannel: releaseChannelOptions[0].value,
    autoUpdate: true,
    forceInstall: false,
    perms: {
      network: true,
      storage: true,
      files: true,
    },
    notes: "",
  };
}

interface ReplacementPreview { requires_provider_confirmation?: boolean; current: { version: string; revision: string; provider: { id: string; authority: string } }; target: { version: string; provider: { id: string; authority: string } } }
interface InstallSession {
  task?: PluginInstallTask;
  taskVisible: boolean;
  creationUncertain: boolean;
  requestID?: string;
  taskInput?: Record<string, unknown>;
  stopTask?: () => void;
  connection: string;
  key: number;
  target?: InstallRecovery['target'];
  state: ReturnType<typeof createInstallForm>;
  plugin?: MarketplacePlugin;
  replacementPreview?: ReplacementPreview;
  providerBindingConfirmed: boolean;
  installing: boolean;
  recovery?: InstallRecovery;
  recoveryError: string;
  checkingRecovery: boolean;
  controller?: AbortController;
  uploadDone?: Promise<void>;
  cancellingUpload: boolean;
  cancelController?: AbortController;
  owner: string;
  creatingDrain: boolean;
  drainRequired: boolean;
  drainCreated: boolean;
  drainPluginId: string;
}

// 一个待核对安装只阻止同一插件；切换目标保留原表单、文件及恢复记录。
const sessions = new Map<string, InstallSession>();
let sessionSequence = 0;
function pendingSession(id: string, except?: InstallSession) {
  return [...sessions.values()].find(session => session !== except && (session.recovery?.target?.id === id || (session.task?.plugin_id === id && !installTaskTerminal(session.task)) || (session.installing && session.target?.id === id)));
}
function sessionFor(plugin?: MarketplacePlugin): InstallSession {
  const key = plugin?.id || '';
  const pending = key ? pendingSession(key) : undefined;
  if (pending) return pending;
  let session = sessions.get(key);
  if (!session) {
    session = reactive({ key: ++sessionSequence, state: createInstallForm(), plugin: plugin ? { ...plugin } : undefined, providerBindingConfirmed: false,
      creationUncertain: false, taskVisible: true, connection: 'idle', installing: false, cancellingUpload: false, recoveryError: '', checkingRecovery: false, owner: '', creatingDrain: false,
      drainRequired: false, drainCreated: false, drainPluginId: '' });
    sessions.set(key, session);
  } else if (!session.recovery) session.plugin = plugin ? { ...plugin } : undefined;
  return session;
}
const currentSession = shallowRef<InstallSession>(sessionFor(props.plugin));
const plugin = computed(() => currentSession.value.plugin);
const state = computed(() => currentSession.value.state);
function sessionField<K extends keyof InstallSession>(key: K) {
  return computed({ get: () => currentSession.value[key], set: (value: InstallSession[K]) => { currentSession.value[key] = value; } });
}
const replacementPreview = sessionField('replacementPreview');
const providerBindingConfirmed = sessionField('providerBindingConfirmed');
const installing = sessionField('installing');
const recovery = sessionField('recovery');
const recoveryError = sessionField('recoveryError');
const checkingRecovery = sessionField('checkingRecovery');
const creatingDrain = sessionField('creatingDrain');
const drainRequired = sessionField('drainRequired');
const drainCreated = sessionField('drainCreated');
const drainPluginId = sessionField('drainPluginId');
let disposed = false;
function abortSessionRequests(session: InstallSession) { session.controller?.abort(); session.cancelController?.abort(); }
const identityEvent = () => { for (const session of sessions.values()) { if (session.owner && session.owner !== installOwner()) { session.stopTask?.(); session.stopTask = undefined; abortSessionRequests(session); session.task = undefined; session.taskInput = undefined; session.requestID = undefined; session.creationUncertain = false; session.state = createInstallForm(); session.recoveryError = t('plugins.installRecovery.sessionChanged'); } }; recentTasks.value = []; };
onMounted(() => window.addEventListener(AUTH_SESSION_EVENT, identityEvent));
onBeforeUnmount(() => window.removeEventListener(AUTH_SESSION_EVENT, identityEvent));
const installOwner = () => `${userStore.user?.id || ''}:${userStore.currentTenantUuid || ''}:${readAuthSession()?.sessionId || ''}`;
const recentTasks = ref<PluginInstallTask[]>([]);
const showTask = computed(() => !!currentSession.value.task && currentSession.value.taskVisible && !recovery.value);
watch(installOwner, (owner) => {
  recentTasks.value = [];
  for (const session of sessions.values()) {
    if (session.owner && session.owner !== owner) {
      session.stopTask?.(); session.stopTask = undefined; abortSessionRequests(session); session.task = undefined; session.taskInput = undefined; session.requestID = undefined; session.creationUncertain = false; session.state = createInstallForm(); session.recoveryError = t('plugins.installRecovery.sessionChanged');
    }
    if (session.recovery && session.owner !== owner) {
      session.controller?.abort();
      session.recovery = { ...session.recovery, outcome: 'changed', inspection: undefined };
      session.recoveryError = t('plugins.installRecovery.sessionChanged');
    }
  }
});
watch(() => [props.plugin?.id, props.plugin?.version], () => {
  abortSessionRequests(currentSession.value); currentSession.value.stopTask?.(); currentSession.value.stopTask = undefined;
  currentSession.value = sessionFor(props.plugin);
  if (props.modelValue) void loadTasks();
});
watch(() => props.modelValue, visible => {
  if (!visible) { abortSessionRequests(currentSession.value); currentSession.value.stopTask?.(); currentSession.value.stopTask = undefined; }
  else void loadTasks();
});
onBeforeUnmount(() => { disposed = true; for (const session of sessions.values()) { abortSessionRequests(session); session.stopTask?.(); } });

function clearRecovery() { recovery.value = undefined; recoveryError.value = ''; }
function viewInstalled() { close(); void router.push('/plugins/local'); }
function completeRecovery() {
  if (recovery.value?.outcome !== 'ready' || currentSession.value.owner !== installOwner()) return;
  notifyMenuRefresh();
  emit('installed', { plugin: plugin.value || null, state: JSON.parse(JSON.stringify(state.value)) });
  clearRecovery(); currentSession.value.task = undefined; close();
}
async function checkInstallation(session = currentSession.value) {
  const current = session.recovery;
  if (disposed || !current?.target || session.checkingRecovery || session.owner !== installOwner()) return;
  session.checkingRecovery = true; session.recoveryError = '';
  session.controller?.abort(); const controller = new AbortController(); session.controller = controller;
  try {
    const { useAdminPluginsService } = await import('~/composables/api/services/adminPluginsService');
    const inspected = await useAdminPluginsService().inspectInstallation(current.target.id, controller.signal);
    if (!controller.signal.aborted && session.owner === installOwner() && session.recovery === current) session.recovery = reconcileInstall(current, inspected);
  } catch (error: any) {
    if (!controller.signal.aborted) session.recoveryError = error?.message || t('plugins.installRecovery.checkFailed');
  } finally { if (session.controller === controller) session.checkingRecovery = false; }
}
async function resumeInstallation() {
  const session = currentSession.value;
  const current = session.recovery;
  if (!current || !canResumeInstall(current) || session.checkingRecovery || session.installing || session.owner !== installOwner()) return;
  session.installing = true; session.recoveryError = '';
  try {
    const { useAdminPluginsService } = await import('~/composables/api/services/adminPluginsService');
    if (disposed || session.owner !== installOwner() || currentSession.value !== session || !props.modelValue) return;
    await useAdminPluginsService().enable(current.target!.id, { expected_version: current.target!.version, expected_revision: current.inspection!.installation.revision! });
    if (!disposed && currentSession.value === session && props.modelValue) await checkInstallation(session);
  } catch (error: any) { session.recoveryError = error?.message || t('plugins.installRecovery.checkFailed'); }
  finally { session.installing = false; }
}
const localDirInputRef = ref<HTMLInputElement | null>(null);
const menuRefreshToken = useState<number>("px-menu-refresh-token", () => 0);
const multipartDirectoryPackageThreshold = 400;

watch(
  () => props.modelValue,
  async (visible) => {
    if (!visible) return;
    const session = currentSession.value;
    if (!userStore.context) {
      try {
        await userStore.fetchUserContext();
      } catch {
        // ignore and fallback to current store snapshot
      }
      if (currentSession.value === session && !session.recovery) session.state.scope = resolveDefaultScope();
    }
  }
);

function close() {
  open.value = false;
}

const drainTargetPluginId = computed(() => drainPluginId.value || plugin.value?.id || "");
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
  const session = currentSession.value;
  const owner = installOwner();
  const id = drainTargetPluginId.value;
  if (!id || session.creatingDrain) return;
  session.creatingDrain = true;
  const toast = useToast();
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    if (disposed || owner !== installOwner() || currentSession.value !== session) return;
    await svc.createDrainJob(id, {
      reason: "install requires plugin drain",
      mode: "drain",
    });
    session.drainCreated = true;
    if (disposed || owner !== installOwner() || currentSession.value !== session) return;
    toast.add({
      title: t("plugins.installDialog.toast.drainCreatedTitle"),
      description: t("plugins.installDialog.toast.drainCreatedDesc"),
      color: "success",
    });
  } catch (error: any) {
    if (disposed || owner !== installOwner() || currentSession.value !== session) return;
    console.error("create drain failed:", error);
    toast.add({
      title: t("plugins.installDialog.toast.drainCreateFailedTitle"),
      description: error?.message || t("plugins.installDialog.toast.drainCreateFailedDesc"),
      color: "error",
    });
  } finally {
    session.creatingDrain = false;
  }
}

function buildInstallMetadataPayload(form = state.value) {
  return {
    scope: form.scope || "system",
    namespace: (form.namespace || "").trim(),
    release_channel: form.releaseChannel || "",
    auto_update: Boolean(form.autoUpdate),
    permissions: {
      network: Boolean(form.perms.network),
      storage: Boolean(form.perms.storage),
      files: Boolean(form.perms.files),
    },
    notes: (form.notes || "").trim(),
  };
}

async function buildLocalInstallFormData(files: File[], directoryName: string, signal?: AbortSignal) {
  signal?.throwIfAborted();
  const formData = new FormData();
  const shouldPackage =
    files.length >= multipartDirectoryPackageThreshold &&
    typeof (globalThis as any).CompressionStream === "function";

  if (shouldPackage) {
    const archive = await createTarGzFromFiles(files, directoryName, signal);
    const name = safeArchiveName(directoryName || "plugin-upload");
    formData.append("file", archive, `${name}.tar.gz`);
    return formData;
  }

  const filePaths: string[] = [];
  for (const file of files) {
    const relPath = (file as any)?.webkitRelativePath || file.name;
    formData.append("files", file, relPath);
    filePaths.push(relPath);
  }
  formData.append("file_paths_json", JSON.stringify(filePaths));
  return formData;
}

function safeArchiveName(name: string) {
  const cleaned = String(name || "")
    .trim()
    .replace(/[^a-zA-Z0-9._-]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return cleaned || "plugin-upload";
}

async function createTarGzFromFiles(files: File[], directoryName: string, signal?: AbortSignal) {
  signal?.throwIfAborted();
  const chunks: BlobPart[] = [];
  const archivePaths = archiveRelativePaths(files);
  for (const file of files) {
    const relPath = archivePaths.get(file) || file.name;
    chunks.push(createTarHeader(relPath, file.size, Math.floor((file.lastModified || Date.now()) / 1000)));
    chunks.push(file);
    const padding = tarPadding(file.size);
    if (padding > 0) {
      chunks.push(new Uint8Array(padding));
    }
  }
  chunks.push(new Uint8Array(1024));

  const tarBlob = new Blob(chunks, { type: "application/x-tar" });
  const CompressionStreamCtor = (globalThis as any).CompressionStream;
  const compressed = tarBlob.stream().pipeThrough(new CompressionStreamCtor("gzip"), { signal });
  const gzipBlob = await new Response(compressed).blob();
  signal?.throwIfAborted();
  return new File([gzipBlob], `${safeArchiveName(directoryName || "plugin-upload")}.tar.gz`, {
    type: "application/gzip",
  });
}

function archiveRelativePaths(files: File[]) {
  const rawPaths = files.map((file) => String((file as any)?.webkitRelativePath || file.name || ""));
  const firstSegments = rawPaths
    .filter((path) => path.includes("/"))
    .map((path) => path.split("/")[0]);
  const commonRoot =
    firstSegments.length === rawPaths.length &&
    firstSegments.every((segment) => segment === firstSegments[0])
      ? firstSegments[0]
      : "";

  const out = new Map<File, string>();
  files.forEach((file, idx) => {
    const raw = rawPaths[idx] || file.name;
    const rel = commonRoot && raw.startsWith(`${commonRoot}/`) ? raw.slice(commonRoot.length + 1) : raw;
    out.set(file, rel || file.name);
  });
  return out;
}

function createTarHeader(path: string, size: number, mtime: number) {
  const header = new Uint8Array(512);
  const { name, prefix } = splitTarPath(path);
  writeTarString(header, 0, 100, name);
  writeTarOctal(header, 100, 8, 0o644);
  writeTarOctal(header, 108, 8, 0);
  writeTarOctal(header, 116, 8, 0);
  writeTarOctal(header, 124, 12, size);
  writeTarOctal(header, 136, 12, mtime);
  for (let i = 148; i < 156; i += 1) {
    header[i] = 0x20;
  }
  header[156] = "0".charCodeAt(0);
  writeTarString(header, 257, 6, "ustar");
  writeTarString(header, 263, 2, "00");
  writeTarString(header, 345, 155, prefix);

  let checksum = 0;
  for (const value of header) {
    checksum += value;
  }
  writeTarOctal(header, 148, 8, checksum);
  return header;
}

function splitTarPath(path: string) {
  const cleaned = String(path || "")
    .replace(/\\/g, "/")
    .replace(/^\/+/, "")
    .replace(/(^|\/)\.\.(?=\/|$)/g, "")
    .replace(/\/+/g, "/");
  const encoded = new TextEncoder().encode(cleaned);
  if (encoded.length <= 100) {
    return { name: cleaned, prefix: "" };
  }

  const parts = cleaned.split("/");
  for (let idx = parts.length - 1; idx > 0; idx -= 1) {
    const name = parts.slice(idx).join("/");
    const prefix = parts.slice(0, idx).join("/");
    if (new TextEncoder().encode(name).length <= 100 && new TextEncoder().encode(prefix).length <= 155) {
      return { name, prefix };
    }
  }
  throw new Error(`uploaded path is too long for tar header: ${cleaned}`);
}

function writeTarString(header: Uint8Array, offset: number, length: number, value: string) {
  const bytes = new TextEncoder().encode(value);
  header.set(bytes.slice(0, length), offset);
}

function writeTarOctal(header: Uint8Array, offset: number, length: number, value: number) {
  const encoded = Math.max(0, value).toString(8).padStart(length - 1, "0").slice(-(length - 1));
  writeTarString(header, offset, length - 1, encoded);
  header[offset + length - 1] = 0;
}

function tarPadding(size: number) {
  const remainder = size % 512;
  return remainder === 0 ? 0 : 512 - remainder;
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
    state.value.localDirFiles = [];
    state.value.localDirName = "";
    target.value = "";
    toast.add({
      title: t("plugins.installDialog.toast.invalidDirectoryTitle"),
      description: t("plugins.installDialog.toast.invalidDirectoryDesc"),
      color: "red",
    });
    return;
  }
  state.value.localDirFiles = files;
  if (files.length > 0) {
    const rel = (files[0] as any)?.webkitRelativePath || "";
    state.value.localDirName = rel ? String(rel).split("/")[0] : t("plugins.installDialog.selectedDirectoryFallback");
  } else {
    state.value.localDirName = "";
  }
}

watch(() => [currentSession.value, state.value.installMode] as const, ([session, mode], [previous]) => { if (session === previous) session.state.forceInstall = mode === 'local'; });
watch(() => [currentSession.value, state.value.url, state.value.installMode, state.value.localDirFiles] as const, ([session], [previous]) => { if (session !== previous) return; session.replacementPreview = undefined; if (session.state.installMode !== 'local') session.state.forceInstall = false; session.providerBindingConfirmed = false; });

function openLocalDirSelector() {
  localDirInputRef.value?.click();
}

function notifyMenuRefresh() {
  menuRefreshToken.value += 1;
}

async function confirmInstall() {
  const session = currentSession.value, form = session.state;
  if (session.installing || session.cancellingUpload || session.recovery || (session.task && session.task.status !== 'awaiting_upload')) return;
  if (session.replacementPreview && (!form.forceInstall || (session.replacementPreview.requires_provider_confirmation && !session.providerBindingConfirmed))) return;
  session.installing = true; session.recoveryError = ''; session.owner = installOwner();
  const owner = session.owner;
  session.controller?.abort(); const controller = new AbortController(); session.controller = controller;
  let finishUpload!: () => void;
  const uploadDone = new Promise<void>(resolve => { finishUpload = resolve; }); session.uploadDone = uploadDone;
  try {
    const { useAdminPluginsService } = await import('~/composables/api/services/adminPluginsService');
    const svc = useAdminPluginsService();
    const target = form.installMode === 'local' ? await readLocalInstallIdentity(form.localDirFiles) : installPackageIdentity(session.plugin?.id, session.plugin?.version);
    if (form.installMode === 'local' && !target) throw new Error(t('plugins.installTask.invalidPackage'));
    if (form.installMode === 'url' && !form.url) throw new Error(t('plugins.installDialog.toast.urlRequired'));
    session.target = target;
    if (target) { const pending = pendingSession(target.id, session); if (pending) { currentSession.value = pending; followTask(pending); return; } }
    if (controller.signal.aborted || owner !== installOwner()) return;
    if (!session.task) {
      session.requestID ||= crypto.randomUUID();
      session.taskInput ||= { request_id: session.requestID, source_kind: form.installMode, plugin_id: target?.id || '', version: target?.version || '',
        url: form.installMode === 'url' ? form.url : undefined, sha256: form.sha256 || undefined, enable: !!form.enableAfterInstall,
        force: !!form.forceInstall, expected_revision: session.replacementPreview?.current.revision,
        confirm_legacy_provider: session.providerBindingConfirmed, metadata: buildInstallMetadataPayload(form) };
      const created = await svc.createInstallation(session.taskInput, controller.signal);
      if (owner !== installOwner()) return;
      session.task = created; session.creationUncertain = false;
    }
    session.taskVisible = true;
    if (session.task.status === 'awaiting_upload') {
      if (session.task.plugin_id !== target?.id || session.task.version !== target?.version) throw new Error(t('plugins.installTask.sourceMismatch'));
      const body = await buildLocalInstallFormData(form.localDirFiles, form.localDirName, controller.signal);
      if (controller.signal.aborted || owner !== installOwner()) return;
      const uploaded = await svc.uploadInstallation(session.task.task_id, body, controller.signal);
      if (controller.signal.aborted || session.cancellingUpload || owner !== installOwner()) return;
      applyTask(session, uploaded);
    }
    if (!disposed && props.modelValue && currentSession.value === session) followTask(session);
  } catch (error: any) {
    if (owner === installOwner() && !session.task && session.taskInput) {
      const status = Number(error?.status || error?.statusCode || error?.response?.status || 0);
      if (status >= 400 && status < 500 && ![408,409].includes(status)) { session.taskInput = undefined; session.requestID = undefined; session.creationUncertain = false; }
      else session.creationUncertain = true;
    }
    if (owner === installOwner() && !controller.signal.aborted) session.recoveryError = error?.message || t('plugins.installTask.requestFailed');
    if (session.task && !session.cancellingUpload && !disposed && props.modelValue && currentSession.value === session) followTask(session);
  } finally {
    if (session.controller === controller) session.installing = false;
    finishUpload();
    if (session.uploadDone === uploadDone) session.uploadDone = undefined;
  }
}

function applyTask(session: InstallSession, next: PluginInstallTask) {
  if (session.owner !== installOwner() || !session.task || !acceptInstallTask(session.task, next)) return;
  session.task = next;
  if (installTaskTerminal(next)) { session.stopTask?.(); session.stopTask = undefined; session.connection = 'idle'; }
}
function followTask(session = currentSession.value) {
  session.stopTask?.(); session.stopTask = undefined; session.connection = 'idle';
  if (disposed || !props.modelValue || session !== currentSession.value || session.owner !== installOwner() || !session.task || installTaskTerminal(session.task)) return;
  const id = session.task.task_id;
  const svc = useAdminPluginsService();
  session.stopTask = connectBackupEvents(signal => svc.installationEvents(id, signal), event => {
    if (event.name === 'status') applyTask(session, event.data);
  }, value => { if (session === currentSession.value && session.owner === installOwner()) session.connection = value; });
}
async function checkTask(session = currentSession.value) {
  if (!session.task || session.checkingRecovery || session.cancellingUpload || session.owner !== installOwner()) return;
  session.checkingRecovery = true; session.recoveryError = '';
  try { const next = await useAdminPluginsService().getInstallationTask(session.task.task_id); applyTask(session, next); followTask(session); }
  catch (error: any) { if (session.owner === installOwner()) session.recoveryError = error?.message || t('plugins.installTask.requestFailed'); }
  finally { session.checkingRecovery = false; }
}
function openTask(task: PluginInstallTask) {
  const session = currentSession.value; session.stopTask?.(); session.task = task; session.owner = installOwner(); session.requestID = task.request_id;
  session.taskVisible = true; session.creationUncertain = false; session.recovery = undefined; session.recoveryError = ''; followTask(session);
}
async function loadTasks() {
  const session = currentSession.value, owner = installOwner();
  try {
    const tasks = await useAdminPluginsService().listInstallationTasks();
    if (disposed || !props.modelValue || currentSession.value !== session || owner !== installOwner()) return;
    recentTasks.value = tasks;
    if (!session.task) {
      const found = tasks.find(item => item.request_id === session.requestID) || (!session.requestID ? tasks.find(item => !installTaskTerminal(item) && (!session.plugin || item.plugin_id === session.plugin.id)) : undefined);
      if (found) openTask(found);
    } else { session.owner = owner; await checkTask(session); }
  } catch (error: any) { if (owner === installOwner()) session.recoveryError = error?.message || t('plugins.installTask.requestFailed'); }
}
async function discardTask() {
  const session = currentSession.value;
  if (!session.task || session.task.status !== 'awaiting_upload' || session.cancellingUpload || session.owner !== installOwner()) return;
  const id = session.task.task_id, owner = session.owner, uploadDone = session.uploadDone;
  const controller = new AbortController(); session.cancelController = controller;
  session.cancellingUpload = true; session.recoveryError = '';
  session.stopTask?.(); session.stopTask = undefined;
  session.controller?.abort();
  const active = () => !disposed && !controller.signal.aborted && owner === installOwner() && session.task?.task_id === id;
  const reconcile = (next: PluginInstallTask) => {
    if (!active()) return;
    applyTask(session, next);
    session.recoveryError = session.task?.status === 'cancelled' ? '' : t(session.task?.status === 'awaiting_upload' ? 'plugins.installTask.cancelUnconfirmed' : 'plugins.installTask.cancelTooLate');
  };
  try {
    await uploadDone;
    if (!active()) return;
    reconcile(await useAdminPluginsService().discardInstallationSource(id, controller.signal));
  } catch {
    if (!active()) return;
    // Reconcile this explicit cancellation once; never poll task status.
    try { reconcile(await useAdminPluginsService().getInstallationTask(id, controller.signal)); }
    catch { if (active()) session.recoveryError = t('plugins.installTask.cancelUnconfirmed'); }
  } finally {
    if (session.cancelController === controller) { session.cancellingUpload = false; session.cancelController = undefined; }
    if (active() && props.modelValue && currentSession.value === session) followTask(session);
  }
}
function reviewTask() {
  const session = currentSession.value, task = session.task; if (!task || !['failed','cancelled'].includes(task.status)) return;
  const details = task.failure_details;
  if (task.error_code === 'PLUGIN_REPLACEMENT_REQUIRED' && details?.current && details?.target) { session.replacementPreview = details as unknown as ReplacementPreview; session.state.forceInstall = false; session.providerBindingConfirmed = false; }
  if (task.error_code === 'PLUGIN_DRAIN_REQUIRED') { session.drainRequired = true; session.drainPluginId = task.plugin_id; }
  session.task = undefined; session.taskInput = undefined; session.requestID = undefined; session.taskVisible = false; session.creationUncertain = false; session.recoveryError = '';
}
async function recoverTask() {
  const session = currentSession.value, task = session.task;
  if (!task || task.status !== 'needs_attention' || !task.installed_revision) return;
  session.stopTask?.(); session.stopTask = undefined;
  session.recovery = { target: { id: task.plugin_id, version: task.version }, receiptRevision: task.installed_revision, enable: true, force: false, outcome: 'unknown' };
  await checkInstallation(session);
}
function completeTask() {
  const session = currentSession.value; if (session.task?.status !== 'succeeded' || session.owner !== installOwner()) return;
  notifyMenuRefresh(); emit('installed', { plugin: session.plugin || null, state: JSON.parse(JSON.stringify(session.state)) });
  session.task = undefined; session.taskInput = undefined; session.requestID = undefined; close();
}

</script>
