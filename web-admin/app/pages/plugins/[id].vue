<template>
  <div class="space-y-6 p-4 sm:p-6">
    <div class="flex items-center justify-between">
      <div class="flex items-start gap-3">
        <img
          v-if="plugin?.icon"
          :src="plugin?.icon"
          alt=""
          class="w-12 h-12 rounded-md object-cover"
        />
        <div>
          <div class="text-xl font-semibold text-[var(--text-primary)]">
            {{ plugin?.name || id }}
          </div>
          <div class="text-sm text-[var(--text-secondary)]">
            版本 {{ plugin?.version || "-" }} · 作者
            {{ plugin?.author || "-" }} · 分类 {{ plugin?.category || "-" }}
          </div>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <UButton
          size="sm"
          variant="ghost"
          icon="i-heroicons-arrow-left"
          :to="'/plugins/market'"
          >返回</UButton
        >
        <UButton
          v-if="isRoot && !sysInstalled"
          size="sm"
          color="primary"
          icon="i-heroicons-arrow-down-tray"
          @click="installOpen = true"
          >安装</UButton
        >
        <UButton
          v-if="isRoot && sysInstalled"
          size="sm"
          variant="outline"
          color="error"
          icon="i-heroicons-trash"
          @click="uninstallPlugin"
          >卸载</UButton
        >
        <!-- 顶部不放启用/停用与刷新，避免与下方系统卡片重复 -->
      </div>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <div class="lg:col-span-2 space-y-6">
        <!-- 介绍 -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-2">介绍</div>
          <p class="text-sm text-[var(--text-secondary)] whitespace-pre-wrap">
            {{ plugin?.description || "-" }}
          </p>
        </div>

        <!-- 版本变更/更新日志（示例） -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-2">
            版本与更新
          </div>
          <ul
            class="list-disc list-inside text-sm text-[var(--text-secondary)] space-y-1"
          >
            <li>v{{ plugin?.version }} 修复若干问题，提升稳定性</li>
            <li>支持更多平台与模型适配</li>
            <li>优化文档与示例</li>
          </ul>
        </div>

        <!-- 权限声明（示例） -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-2">权限</div>
          <div class="text-sm text-[var(--text-secondary)] space-y-1">
            <div>• 网络访问（请求外部 API）</div>
            <div>• 本地存储（读写插件数据）</div>
            <div>• 文件访问（读取/上传文件）</div>
          </div>
        </div>
      </div>

      <div class="space-y-6">
        <!-- 侧边信息 -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-3">
            统计与标签
          </div>
          <div class="text-sm text-[var(--text-secondary)]">
            安装量：{{ formatCount(plugin?.installs || 0) }}
          </div>
          <div class="mt-2 flex flex-wrap gap-2">
            <UBadge
              v-for="t in plugin?.tags || []"
              :key="t"
              variant="soft"
              size="xs"
              >{{ t }}</UBadge
            >
          </div>
        </div>

        <!-- 系统控制 -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-3">
            系统运行
          </div>
          <div class="text-sm text-[var(--text-secondary)] space-y-2">
            <div>
              系统启用：<UBadge
                :color="sysEnabled ? 'green' : 'neutral'"
                size="xs"
                >{{ sysEnabled ? "是" : "否" }}</UBadge
              >
            </div>
            <div>状态：{{ sysStatus || "-" }}</div>
            <div class="flex flex-wrap items-center gap-2 mt-2">
              <UButton
                v-if="isRoot"
                size="sm"
                :variant="sysEnabled ? 'outline' : 'solid'"
                :color="sysEnabled ? 'error' : 'primary'"
                :icon="sysEnabled ? 'i-heroicons-pause' : 'i-heroicons-play'"
                @click="toggleEnable"
              >
                {{ sysEnabled ? "停用" : "启用" }}
              </UButton>
              <UButton
                v-if="isRoot && sysInstalled"
                size="sm"
                variant="outline"
                icon="i-heroicons-arrow-path"
                @click="restartPlugin"
                >重启</UButton
              >
              <UButton
                v-if="isRoot && sysInstalled"
                size="sm"
                variant="outline"
                icon="i-heroicons-arrow-up-on-square"
                @click="switchVersion"
                >切换版本</UButton
              >
              <UButton
                size="sm"
                variant="ghost"
                icon="i-heroicons-clipboard-document-list"
                @click="openLogs"
                >查看日志</UButton
              >
              <UButton
                size="sm"
                variant="ghost"
                icon="i-heroicons-arrow-path"
                @click="refreshStatus"
                >刷新状态</UButton
              >
            </div>
          </div>
        </div>

        <!-- 租户控制（仅当前租户管理员可见） -->
        <div
          class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
        >
          <div class="font-medium text-[var(--text-primary)] mb-3">本租户</div>
          <div class="text-sm text-[var(--text-secondary)] space-y-2">
            <div>
              启用：
              <UBadge :color="tenantEnabled ? 'green' : 'neutral'" size="xs">{{
                tenantEnabled ? "是" : "否"
              }}</UBadge>
            </div>
            <div v-if="clientId">
              client_id：<code>{{ clientId }}</code>
            </div>
            <div class="flex flex-wrap items-center gap-2 mt-2">
              <UButton
                v-if="isTenantAdmin"
                size="sm"
                :variant="tenantEnabled ? 'outline' : 'solid'"
                :color="tenantEnabled ? 'error' : 'primary'"
                :icon="tenantEnabled ? 'i-heroicons-pause' : 'i-heroicons-play'"
                @click="toggleTenant"
              >
                {{ tenantEnabled ? "停用本租户" : "启用本租户" }}
              </UButton>
              <UButton
                v-if="isTenantAdmin && tenantEnabled"
                size="sm"
                variant="outline"
                icon="i-heroicons-key"
                @click="rotateTenantSecret"
                >轮换密钥</UButton
              >
              <UButton
                v-if="isTenantAdmin"
                size="sm"
                variant="outline"
                color="error"
                icon="i-heroicons-trash"
                @click="deleteTenantConfig"
                >删除配置</UButton
              >
              <UButton
                size="sm"
                variant="ghost"
                icon="i-heroicons-arrow-path"
                @click="refreshTenant"
                >刷新</UButton
              >
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 安装对话框 -->
    <InstallDialog
      :model-value="installOpen"
      @update:modelValue="(v) => (installOpen.value = v)"
      :plugin="plugin"
      @installed="onInstalled"
    />
  </div>
</template>

<script setup lang="ts">
import InstallDialog from "~/components/plugins/InstallDialog.vue";
import type { MarketplacePlugin } from "~/components/plugins/PluginCard.vue";
import { useUserStore } from "~/stores/user";
import {
  LazyPluginsLogsModal,
  LazyPluginsSwitchVersionModal,
} from "#components";

definePageMeta({
  layout: "default",
});

const route = useRoute();
const id = computed(() => String(route.params.id || ""));
const plugin = ref<MarketplacePlugin | undefined>(undefined);

const installOpen = ref(false);

// 系统状态
const sysEnabled = ref<boolean>(false);
const sysInstalled = ref<boolean>(false);
const sysStatus = ref<string>("");
const tenantEnabled = ref<boolean>(false);
const clientId = ref<string>("");

const showSecret = ref(false);
const oneTimeSecret = ref<string>("");

async function refreshStatus() {
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const s: any = await svc.status(id.value);
    sysStatus.value = typeof s === "string" ? s : s?.state || s?.status || "";
    // 如果 marketplace 提供了 isSystemEnabled，可补充；否则从状态推断
    sysEnabled.value = Boolean(
      s?.enabled ??
        s?.isSystemEnabled ??
        (sysStatus.value && sysStatus.value !== "disabled")
    );
  } catch (e) {
    console.warn("load status failed:", e);
  }
}

async function toggleEnable() {
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();

    if (sysEnabled.value) {
      // 停用时需要确认
      const { useConfirm } = await import("~/composables/useConfirm");
      const { confirm } = useConfirm();
      const ok = await confirm({
        title: "停用插件",
        description: "停用后该插件将无法为任何租户提供服务。",
        message: "确定要停用该插件吗？",
        confirmLabel: "停用",
        cancelLabel: "取消",
        tone: "warning",
      });
      if (!ok) return;
      await svc.disable(id.value);
    } else {
      // 启用时直接执行
      await svc.enable(id.value);
    }

    await refreshStatus();
    await refreshMeta();
  } catch (e) {
    console.error("toggle enable failed:", e);
  }
}

onMounted(async () => {
  // 加载详情（从 marketplace v2 里筛一条）
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const list = await svc.getMarketplace();
    const item = Array.isArray(list)
      ? (list as any[]).find(
          (p) => String(p.id || p.slug || p.name || "") === id.value
        )
      : undefined;
    if (item) {
      plugin.value = {
        id: String(item.id || item.slug || item.name || ""),
        name: item.name || item.id || "-",
        description: item.description || "",
        version: item.version || "-",
        author: item.author || "",
        category: item.category || "",
        installs: Number(item.installs || item.downloadCount || 0),
        icon: item.icon,
        tags: Array.isArray(item.tags) ? item.tags : [],
      };
      sysInstalled.value = !!(item as any).isSystemInstalled;
      if ((item as any).isSystemEnabled !== undefined) {
        sysEnabled.value = !!(item as any).isSystemEnabled;
      }
    }
  } catch (e) {
    console.warn("load plugin detail failed:", e);
  }
  await refreshStatus();
  await refreshTenant();
});

async function refreshMeta() {
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const list = await svc.getMarketplace();
    const item = Array.isArray(list)
      ? (list as any[]).find(
          (p) => String(p.id || p.slug || p.name || "") === id.value
        )
      : undefined;
    if (item) {
      sysInstalled.value = !!(item as any).isSystemInstalled;
      if ((item as any).isSystemEnabled !== undefined)
        sysEnabled.value = !!(item as any).isSystemEnabled;
    }
  } catch (e) {
    console.warn("refresh meta failed:", e);
  }
}

async function refreshTenant() {
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const conf: any = await svc.getTenantConfig(id.value);
    tenantEnabled.value = Boolean(conf?.enabled ?? conf?.isEnabled);
    clientId.value = conf?.client_id || conf?.clientId || clientId.value || "";
  } catch (e) {
    console.warn("load tenant config failed:", e);
  }
}

async function toggleTenant() {
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    if (tenantEnabled.value) {
      const { useConfirm } = await import("~/composables/useConfirm");
      const { confirm } = useConfirm();
      const ok = await confirm({
        title: "停用本租户",
        description: "仅影响当前租户的访问，其他租户不受影响。",
        message: "确定要停用本租户对该插件的访问吗？",
        confirmLabel: "停用",
        cancelLabel: "取消",
        tone: "warning",
      });
      if (!ok) return;
      await svc.setTenantEnabled(id.value, false);
      tenantEnabled.value = false;
    } else {
      const resp: any = await svc.setTenantEnabled(id.value, true);
      // 首次启用可能返回一次性明文 secret
      const secret = resp?.client_secret || resp?.secret || "";
      const cid = resp?.client_id || resp?.clientId;
      if (cid) clientId.value = cid;
      if (secret) {
        oneTimeSecret.value = secret;
        showSecret.value = true;
      }
      tenantEnabled.value = true;
    }
  } catch (e) {
    console.error("toggle tenant failed:", e);
  }
}

async function rotateTenantSecret() {
  const { useConfirm } = await import("~/composables/useConfirm");
  const { confirm } = useConfirm();
  const ok = await confirm({
    title: "轮换密钥",
    description: "旧密钥将立即失效，请及时更新插件端配置。",
    message: "确定要为本租户轮换密钥吗？",
    confirmLabel: "轮换",
    cancelLabel: "取消",
    tone: "warning",
  });
  if (!ok) return;
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const resp: any = await svc.rotateCredentials(id.value);
    const secret = resp?.client_secret || resp?.secret || "";
    const cid = resp?.client_id || resp?.clientId;
    if (cid) clientId.value = cid;
    if (secret) {
      const { confirm: info } = useConfirm();
      await info({
        title: "新密钥（仅此一次展示）",
        message: secret,
        confirmLabel: "已复制",
        cancelLabel: "",
        tone: "info",
      });
    }
  } catch (e) {
    console.error("rotate credentials failed:", e);
  }
}

async function deleteTenantConfig() {
  const { useConfirm } = await import("~/composables/useConfirm");
  const { confirm } = useConfirm();
  const ok = await confirm({
    title: "删除本租户配置",
    description: "删除后本租户将无法访问该插件，需重新启用生成新密钥。",
    message: "确定删除本租户配置吗？",
    confirmLabel: "删除",
    cancelLabel: "取消",
    tone: "danger",
  });
  if (!ok) return;
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    await svc.deleteTenantConfig(id.value);
    tenantEnabled.value = false;
    clientId.value = "";
  } catch (e) {
    console.error("delete tenant config failed:", e);
  }
}

async function restartPlugin() {
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    await svc.restart(id.value);
    await refreshStatus();
  } catch (e) {
    console.error("restart failed:", e);
  }
}

async function switchVersion() {
  const { useOverlay } = await import("#imports");
  const overlay = useOverlay();
  const modal = overlay.create(LazyPluginsSwitchVersionModal);
  const instance = modal.open({
    pluginId: id.value,
    currentVersion: plugin.value?.version,
  });
  const version = await instance.result;
  if (!version) return;
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    await svc.switchVersion(id.value, version, { enable: true });
    await refreshMeta();
    await refreshStatus();
  } catch (e) {
    console.error("switch version failed:", e);
  }
}

function formatCount(n: number) {
  if (n >= 10000) return (n / 10000).toFixed(1) + "w";
  if (n >= 1000) return (n / 1000).toFixed(1) + "k";
  return String(n);
}

async function onInstalled(_payload?: {
  plugin: MarketplacePlugin | null;
  state: any;
}) {
  try {
    installOpen.value = false;
    await refreshMeta();
    await refreshStatus();
  } catch (e) {
    console.error("Installed refresh failed:", e);
  }
}

// 角色
const userStore = useUserStore();
const isRoot = computed(() => userStore.isRoot);
const isTenantAdmin = computed(() => userStore.isCurrentTenantAdmin);

async function uninstallPlugin() {
  const { useConfirm } = await import("~/composables/useConfirm");
  const { confirm } = useConfirm();
  const ok = await confirm({
    title: "卸载插件",
    description: "此操作将影响所有租户，且可能中断服务访问。",
    message: "确定卸载该插件？卸载将影响所有租户。",
    confirmLabel: "卸载",
    cancelLabel: "取消",
    tone: "danger",
  });
  if (!ok) return;
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    await svc.uninstall(id.value);
    await refreshMeta();
    await refreshStatus();
  } catch (e) {
    console.error("uninstall failed:", e);
  }
}

async function openLogs() {
  const { useOverlay } = await import("#imports");
  const overlay = useOverlay();
  const modal = overlay.create(LazyPluginsLogsModal);
  modal.open({ pluginId: id.value });
}
</script>
