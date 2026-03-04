<template>
  <UModal
    v-model:open="open"
    :title="plugin ? '安装插件' : '安装插件'"
    :description="plugin ? plugin.name || '' : '选择安装来源：远程URL或本地上传'"
    :ui="{ width: 'sm:max-w-lg' }"
  >
    <template #content>
      <div class="p-4">
        <div class="flex items-start gap-3">
          <img
            v-if="plugin?.icon"
            :src="plugin?.icon"
            alt=""
            class="w-10 h-10 rounded-md object-cover"
          />
          <div class="flex-1">
            <div class="font-medium text-[var(--text-primary)]">
              {{ plugin ? `安装插件：${plugin?.name}` : "安装插件" }}
            </div>
            <div v-if="plugin" class="text-xs text-[var(--text-secondary)]">
              版本：{{ plugin?.version || "-" }} · 作者：{{
                plugin?.author || "-"
              }}
            </div>
          </div>
        </div>

        <div class="mt-4">
          <UForm :state="state" class="space-y-4">
            <!-- 安装来源选择 -->
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm mb-1 text-[var(--text-secondary)]"
                  >安装方式</label
                >
                <USelect
                  v-model="state.installMode"
                  :items="['远程URL', '本地上传']"
                />
              </div>
              <div class="flex items-center gap-3 mt-6 md:mt-8">
                <USwitch v-model="state.enableAfterInstall" />
                <span class="text-sm text-[var(--text-secondary)]"
                  >安装后立即启用</span
                >
              </div>
            </div>

            <!-- 安装来源：URL（对接后端 install/url） -->
            <div
              class="rounded-md border border-[var(--border-color)] p-3 space-y-3"
            >
              <div class="text-sm font-medium text-[var(--text-primary)]">
                安装包来源（支持 px-plugin build 产物）
              </div>
              <UInput
                v-if="state.installMode === '远程URL'"
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
                v-if="state.installMode === '远程URL'"
                v-model="state.sha256"
                placeholder="可选：期望的 SHA256 校验值"
              />

              <div v-if="state.installMode === '本地上传'" class="space-y-2">
                <UButton icon="i-heroicons-folder-open" color="primary" variant="solid" @click="openLocalDirSelector">
                  选择本地来源
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
                  已选择目录：{{ state.localDirName }}（{{ state.localDirFiles.length }} 个文件）
                </div>
              </div>

              <div class="text-xs text-[var(--text-secondary)]">
                本地上传：请选择插件目录。目录内包含 <code>plugin.yaml</code> 或
                <code>*.tar.gz/.tgz</code> 即可安装。
              </div>
            </div>
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm mb-1 text-[var(--text-secondary)]"
                  >安装范围</label
                >
                <USelect
                  v-model="state.scope"
                  :items="scopeOptions"
                  icon="i-heroicons-cog-6-tooth"
                />
              </div>
              <div>
                <label class="block text-sm mb-1 text-[var(--text-secondary)]"
                  >命名空间（可选）</label
                >
                <UInput
                  v-model="state.namespace"
                  placeholder="如: company.crm"
                />
              </div>
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label class="block text-sm mb-1 text-[var(--text-secondary)]"
                  >目标环境</label
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
                  >自动更新</span
                >
              </div>
            </div>

            <div>
              <div class="text-sm mb-2 text-[var(--text-secondary)]">
                申请权限
              </div>
              <div
                class="rounded-md border border-[var(--border-color)] p-3 space-y-2"
              >
                <label class="flex items-center gap-2 text-sm">
                  <UCheckbox v-model="state.perms.network" />
                  <span>网络访问（调用外部 API）</span>
                </label>
                <label class="flex items-center gap-2 text-sm">
                  <UCheckbox v-model="state.perms.storage" />
                  <span>本地存储（读写插件数据）</span>
                </label>
                <label class="flex items-center gap-2 text-sm">
                  <UCheckbox v-model="state.perms.files" />
                  <span>文件访问（读取/上传文件）</span>
                </label>
              </div>
            </div>

            <div>
              <label class="block text-sm mb-1 text-[var(--text-secondary)]"
                >备注（可选）</label
              >
              <UTextarea
                v-model="state.notes"
                :rows="3"
                placeholder="为本次安装记录备注…"
              />
            </div>
          </UForm>
        </div>

        <div class="mt-5 flex items-center justify-end gap-2">
          <UButton variant="ghost" @click="close">取消</UButton>
          <UButton
            color="primary"
            :loading="installing"
            @click="confirmInstall"
            icon="i-heroicons-arrow-down-tray"
            >安装</UButton
          >
        </div>
      </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import { storeToRefs } from "pinia";
import type { MarketplacePlugin } from "~/components/plugins/PluginCard.vue";

const props = defineProps<{
  modelValue: boolean;
  plugin?: MarketplacePlugin;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", v: boolean): void;
  // ✅ 允许 plugin 为空，兼容“通用安装”
  (
    e: "installed",
    payload: { plugin: MarketplacePlugin | null; state: any }
  ): void;
}>();

// ✅ 补上：给模板里用的 plugin
const plugin = computed(() => props.plugin);

// 保留你的 v-model:open 写法，但这是 setter（仅在我们主动赋值时触发）
const open = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit("update:modelValue", v),
});

const userStore = useUserStore();
const { isRoot, isCurrentTenantAdmin } = storeToRefs(userStore);

function resolveDefaultScope(): "system" | "org" | "user" {
  if (isRoot.value) return "system";
  if (isCurrentTenantAdmin.value) return "org";
  return "user";
}

const scopeOptions = [
  { label: "系统级", value: "system" },
  { label: "组织级", value: "org" },
  { label: "用户级", value: "user" },
];
const envOptions = [
  { label: "default", value: "default" },
  { label: "staging", value: "staging" },
  { label: "production", value: "production" },
];

const state = reactive({
  installMode: "远程URL" as "远程URL" | "本地上传",
  url: "",
  sha256: "",
  enableAfterInstall: true,
  localDirFiles: [] as File[],
  localDirName: "",
  scope: resolveDefaultScope(),
  namespace: "",
  env: envOptions[0].value,
  autoUpdate: true,
  perms: {
    network: true,
    storage: true,
    files: false,
  },
  notes: "",
});

const installing = ref(false);
const localDirInputRef = ref<HTMLInputElement | null>(null);

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
      title: "目录无效",
      description: "目录内需包含 plugin.yaml 或 *.tar.gz/.tgz",
      color: "red",
    });
    return;
  }
  state.localDirFiles = files;
  if (files.length > 0) {
    const rel = (files[0] as any)?.webkitRelativePath || "";
    state.localDirName = rel ? String(rel).split("/")[0] : "已选目录";
  } else {
    state.localDirName = "";
  }
}

function openLocalDirSelector() {
  localDirInputRef.value?.click();
}

async function confirmInstall() {
  // 检查是否有有效的安装来源
  if (state.installMode === "远程URL" && !state.url) {
    const toast = useToast();
    toast.add({
      title: "错误",
      description: "请填写插件的远程URL地址",
      color: "red",
    });
    return;
  }
  if (
    state.installMode === "本地上传" &&
    state.localDirFiles.length === 0
  ) {
    const toast = useToast();
    toast.add({
      title: "错误",
      description: "请选择插件目录",
      color: "red",
    });
    return;
  }

  installing.value = true;
  const toast = useToast();

  try {
    if (state.installMode === "远程URL" && state.url) {
      const { useAdminPluginsService } = await import(
        "~/composables/api/services/adminPluginsService"
      );
      const svc = useAdminPluginsService();
      await svc.installFromUrl({
        url: state.url,
        sha256: state.sha256 || undefined,
        enable: !!state.enableAfterInstall,
        metadata: buildInstallMetadataPayload(),
      });

      toast.add({ title: "成功", description: "插件安装成功", color: "green" });
    } else if (
      state.installMode === "本地上传" &&
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
      }
      formData.append("enable", String(!!state.enableAfterInstall));
      formData.append("metadata", JSON.stringify(buildInstallMetadataPayload()));
      await svc.installFromLocal(formData);

      toast.add({ title: "成功", description: "插件安装成功", color: "green" });
    } else {
      // 占位：没有填写来源时不做请求
      toast.add({
        title: "提示",
        description: "未填写 URL 或未选择本地目录，未发起安装请求",
        color: "warning",
      });
      return;
    }

    emit("installed", {
      plugin: props.plugin || null,
      state: JSON.parse(JSON.stringify(state)),
    });
    close();
  } catch (error: any) {
    console.error("插件安装失败:", error);
    toast.add({
      title: "安装失败",
      description: error?.message || "插件安装过程中发生错误，请重试",
      color: "error",
    });
  } finally {
    installing.value = false;
  }
}
</script>
