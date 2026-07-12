<template>
  <div class="space-y-6 p-4">
    <div class="flex items-center justify-between gap-3">
      <div v-if="isRoot" class="flex items-center gap-2">
        <UButton variant="ghost" size="sm" :to="'/plugins/market'">
          {{ marketTabLabel }}
        </UButton>
        <UButton :variant="'solid'" size="sm" :to="'/plugins/installed'">
          {{ installedTabLabel }}
        </UButton>
      </div>
      <h1 v-else class="text-lg font-semibold text-[var(--text-primary)]">
        插件订阅
      </h1>
      <div class="flex items-center gap-2">
        <UInput v-model="q" placeholder="搜索名称/描述/作者…" class="w-64" />
        <UButton
          v-if="isRoot"
          size="sm"
          icon="i-heroicons-arrow-down-tray"
          @click="openInstallGeneric"
          >安装</UButton
        >
        <UButton
          icon="i-heroicons-arrow-path"
          variant="ghost"
          size="sm"
          @click="refresh"
          >刷新</UButton
        >
      </div>
    </div>

    <UCard v-if="filtered.length > 0" :ui="{ body: { padding: 'p-0' } }">
      <div class="overflow-x-auto">
        <table class="min-w-full text-sm">
          <thead
            class="bg-gray-50 dark:bg-gray-800/40 border-b border-[var(--border-color)]"
          >
            <tr>
              <th class="text-left px-4 py-2 w-[24%]">插件</th>
              <th class="text-left px-4 py-2 w-[10%]">版本</th>
              <th v-if="isRoot" class="text-left px-4 py-2 w-[15%]">系统状态</th>
              <th class="text-left px-4 py-2 w-[12%]">
                {{ isRoot ? "租户状态" : "订阅状态" }}
              </th>
              <th class="text-left px-4 py-2 w-[15%]">客户端ID</th>
              <th class="text-left px-4 py-2 w-[24%]">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="it in filtered"
              :key="it.id"
              class="border-b border-[var(--border-color)]"
            >
              <td class="px-4 py-3">
                <div class="flex items-center gap-3">
                  <img
                    v-if="it.icon"
                    :src="it.icon"
                    alt=""
                    class="w-8 h-8 rounded object-cover"
                  />
                  <div>
                    <NuxtLink
                      :to="`/plugins/${it.id}`"
                      class="font-medium hover:underline"
                      >{{ it.name || it.id }}</NuxtLink
                    >
                    <div
                      class="text-xs text-[var(--text-secondary)] line-clamp-1"
                    >
                      {{ it.description }}
                    </div>
                  </div>
                </div>
              </td>
              <td class="px-4 py-3">{{ it.version || "-" }}</td>
              <td v-if="isRoot" class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <UBadge
                    :color="it.isSystemEnabled ? 'green' : 'neutral'"
                    size="xs"
                    >{{ it.isSystemEnabled ? "已启用" : "未启用" }}</UBadge
                  >
                  <span class="text-xs text-[var(--text-secondary)]">{{
                    it.systemStatus || "-"
                  }}</span>
                </div>
              </td>
              <td class="px-4 py-3">
                <UBadge
                  :color="it.tenantEnabled ? 'green' : 'neutral'"
                  size="xs"
                  >{{ it.tenantEnabled ? "已订阅" : "未订阅" }}</UBadge
                >
              </td>
              <td class="px-4 py-3">
                <span
                  v-if="it.clientId"
                  class="text-xs text-[var(--text-secondary)] font-mono"
                  >{{ it.clientId }}</span
                >
                <span v-else class="text-xs text-[var(--text-secondary)]"
                  >-</span
                >
              </td>
              <td class="px-4 py-3">
                <div class="flex items-center gap-2">
                  <!-- 系统级启用/停用按钮（root用户） -->
                  <UButton
                    v-if="isRoot && it.isSystemInstalled"
                    size="sm"
                    :variant="it.isSystemEnabled ? 'outline' : 'solid'"
                    :color="it.isSystemEnabled ? 'error' : 'primary'"
                    :icon="
                      it.isSystemEnabled
                        ? 'i-heroicons-pause'
                        : 'i-heroicons-play'
                    "
                    @click="toggleEnable(it)"
                  >
                    {{ it.isSystemEnabled ? "停用" : "启用" }}
                  </UButton>

                  <!-- 租户级启用/停用按钮（租户管理员） -->
                  <UButton
                    v-if="isTenantAdmin && !isRoot && it.tenantEnabled"
                    size="sm"
                    :variant="it.tenantEnabled ? 'outline' : 'solid'"
                    :color="it.tenantEnabled ? 'error' : 'primary'"
                    :icon="
                      it.tenantEnabled
                        ? 'i-heroicons-pause'
                        : 'i-heroicons-play'
                    "
                    @click="toggleTenant(it)"
                  >
                    {{ it.tenantEnabled ? "取消订阅" : "订阅启用" }}
                  </UButton>

                  <!-- 更多操作按钮 -->
                  <UButton
                    size="sm"
                    variant="ghost"
                    icon="i-heroicons-ellipsis-horizontal"
                    :to="`/plugins/${it.id}`"
                  >
                    更多
                  </UButton>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </UCard>

    <div
      v-else
      class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] py-16 text-center"
    >
      <UIcon
        name="i-heroicons-archive-box"
        class="mx-auto mb-4 size-10 text-[var(--text-secondary)]"
      />
      <h3 class="text-lg font-medium text-[var(--text-primary)]">
        {{ isRoot ? "暂无已安装插件" : "暂无订阅插件" }}
      </h3>
      <p class="mt-2 text-sm text-[var(--text-secondary)]">
        {{ isRoot ? "安装插件后会显示在这里。" : "从可订阅插件中启用后会显示在这里。" }}
      </p>
    </div>

    <!-- 安装对话框 -->
    <InstallDialog
      v-model="installOpen"
      :plugin="selectedPlugin"
      @installed="onInstalled"
    />
  </div>
</template>

<script setup lang="ts">
import { useUserStore } from "~/stores/user";
import InstallDialog from "~/components/plugins/InstallDialog.vue";

definePageMeta({ layout: "default" });

type Row = any;

const userStore = useUserStore();
const isRoot = computed(() => userStore.isRoot);
const isTenantAdmin = computed(() => userStore.isCurrentTenantAdmin);
const marketTabLabel = computed(() => "插件市场");
const installedTabLabel = computed(() => "已安装");

const q = ref("");
const rows = ref<Row[]>([]);
const installOpen = ref(false);
const selectedPlugin = ref<any>(undefined);
const menuRefreshToken = useState<number>("px-menu-refresh-token", () => 0);

function openInstallGeneric() {
  selectedPlugin.value = undefined; // 通用安装时允许为 undefined/null
  installOpen.value = true;
}

const filtered = computed(() => {
  const s = q.value.trim().toLowerCase();
  if (!s) return rows.value;
  return rows.value.filter((r) =>
    [r.name, r.id, r.description, r.author].some((v: any) =>
      String(v || "")
        .toLowerCase()
        .includes(s)
    )
  );
});

async function load() {
  const { useAdminPluginsService } = await import(
    "~/composables/api/services/adminPluginsService"
  );
  const svc = useAdminPluginsService();
  const list = isRoot.value ? await svc.list() : await svc.getMarketplace();
  const mapped = (list || []).map((p: any) => ({
    id: String(p.id || p.slug || p.name || ""),
    name: (p.menus && p.menus[0]?.title) || p.name || p.id || "-",
    description: p.description || "",
    author: p.author || "",
    version: p.version || "-",
    icon: p.icon,
    isSystemInstalled: isRoot.value ? true : !!p.isSystemInstalled,
    isSystemEnabled: isRoot.value
      ? String(p.state || "").toLowerCase() === "enabled"
      : !!p.isSystemEnabled,
    systemStatus: isRoot.value ? p.state || "" : p.systemStatus || "",
    tenantEnabled: !!(p.tenantInstance?.enabled ?? p.tenantEnabled),
    clientId:
      p.tenantInstance?.config?.client_id ||
      p.tenantInstance?.config?.clientId ||
      "",
  }));
  rows.value = mapped;
  if (isRoot.value) {
    await Promise.all(
      rows.value.map(async (r) => {
        try {
          const conf: any = await svc.getTenantConfig(r.id);
          r.tenantEnabled = !!(conf?.enabled ?? conf?.isEnabled);
          r.clientId =
            conf?.client_id || conf?.clientId || conf?.config?.client_id || "";
        } catch {}
      })
    );
  } else {
    rows.value = rows.value.filter((r) => r.isSystemEnabled && r.tenantEnabled);
  }
}

onMounted(load);

function refresh() {
  load();
}

// ✅ 这里允许 payload 为 undefined/null，且加了兜底，避免 “Unhandled error …”
async function onInstalled(_payload?: { plugin: any | null; state: any }) {
  try {
    installOpen.value = false;
    await load();
  } catch (e) {
    console.error("[installed] refresh after install failed:", e);
  }
}

async function toggleEnable(r: Row) {
  const { useAdminPluginsService } = await import(
    "~/composables/api/services/adminPluginsService"
  );
  const svc = useAdminPluginsService();
  if (r.isSystemEnabled) await svc.disable(r.id);
  else await svc.enable(r.id);
  menuRefreshToken.value += 1;
  await load();
}

async function toggleTenant(r: Row) {
  const { useAdminPluginsService } = await import(
    "~/composables/api/services/adminPluginsService"
  );
  const svc = useAdminPluginsService();
  if (r.tenantEnabled) {
    const { useConfirm } = await import("~/composables/useConfirm");
    const { confirm } = useConfirm();
    const ok = await confirm({
      title: "停用本租户",
      description: "仅取消当前租户订阅，不会停止平台插件进程。",
      message: `取消订阅 ${r.name}？`,
      confirmLabel: "取消订阅",
      tone: "warning",
    });
    if (!ok) return;
    await svc.setTenantEnabled(r.id, false);
    r.tenantEnabled = false;
  } else {
    const resp: any = await svc.setTenantEnabled(r.id, true);
    r.tenantEnabled = true;
    r.clientId = resp?.client_id || resp?.clientId || resp?.instance?.config?.client_id || r.clientId;
  }
}
</script>
