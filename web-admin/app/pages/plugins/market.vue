<template>
  <div class="space-y-6 p-4">
    <div class="flex items-center justify-between gap-3">
      <div v-if="isRoot" class="flex items-center gap-2">
        <UButton :variant="'solid'" size="sm" :to="'/plugins/market'">
          {{ marketTabLabel }}
        </UButton>
        <UButton variant="ghost" size="sm" :to="'/plugins/installed'">
          {{ installedTabLabel }}
        </UButton>
      </div>
      <h1 v-else class="text-lg font-semibold text-[var(--text-primary)]">
        插件订阅
      </h1>
      <div class="flex items-center gap-2">
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

    <!-- 筛选区 -->
    <div
      class="rounded-lg border border-[var(--border-color)] bg-[var(--card-bg)] p-4"
    >
      <div class="grid grid-cols-1 md:grid-cols-4 gap-3">
        <UInput
          v-model="q"
          placeholder="搜索插件名称/描述/作者…"
          icon="i-heroicons-magnifying-glass"
        />
        <USelect
          v-model="category"
          :items="categoryOptions"
          icon="i-heroicons-squares-2x2"
        />
        <USelect
          v-model="status"
          :items="statusOptions"
          icon="i-heroicons-sparkles"
        />
        <USelect
          v-model="sort"
          :items="sortOptions"
          icon="i-heroicons-adjustments-vertical"
        />
      </div>
    </div>

    <!-- 列表区 -->
    <div class="space-y-6">
      <!-- 结果统计 + 调试：确认分页区间 -->
      <div
        class="flex items-center justify-between text-sm text-[var(--text-secondary)]"
      >
        <span>共找到 {{ filtered.length }} 个插件</span>
        <span>
          第 {{ currentPage }} / {{ totalPages }} 页
          <span class="ml-2 text-xs opacity-60">
            [{{ pageStart }}-{{ pageEnd - 1 }}] 本页
            {{ paginatedData.length }} 个
          </span>
        </span>
      </div>

      <!-- 插件网格：确保使用 paginatedData -->
      <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
        <PluginCard
          v-for="p in paginatedData"
          :key="p.id"
          :plugin="p"
          :is-system-installed="Boolean((p as any).__sys?.isSystemInstalled)"
          :is-system-enabled="Boolean((p as any).__sys?.isSystemEnabled)"
          :system-status="String((p as any).__sys?.systemStatus || '')"
          :is-tenant-enabled="Boolean((p as any).__tenant?.enabled)"
          :tenant-status="String((p as any).__tenant?.status || '')"
          :can-install="isRoot"
          :can-manage-tenant="isTenantAdmin"
          :show-system-state="isRoot"
          :can-manage-detail="isRoot || Boolean((p as any).__tenant?.enabled)"
          @install="openInstall"
          @toggle-tenant="toggleTenant"
        />
      </div>

      <!-- 空状态 -->
      <div
        v-if="filtered.length === 0"
        class="flex flex-col items-center justify-center py-12 text-center"
      >
        <div
          class="w-16 h-16 mx-auto mb-4 bg-gray-100 rounded-full flex items-center justify-center"
        >
          <UIcon
            name="i-heroicons-puzzle-piece"
            class="w-8 h-8 text-gray-400"
          />
        </div>
        <h3 class="text-lg font-medium text-[var(--text-primary)] mb-2">
          {{ isRoot ? "未找到相关插件" : "未找到可订阅插件" }}
        </h3>
        <p class="text-[var(--text-secondary)] max-w-sm">
          {{
            isRoot
              ? "尝试调整搜索条件或浏览其他分类"
              : "当前没有匹配的插件订阅项"
          }}
        </p>
      </div>

      <!-- 分页控件：Nuxt UI v3 正确写法 -->
      <div v-if="totalPages > 1" class="flex justify-center">
        <UPagination
          v-model:page="currentPage"
          :items-per-page="pageSize"
          :total="filtered.length"
          :sibling-count="1"
          show-edges
          :ui="{
            wrapper: 'flex items-center gap-1',
            rounded: '!rounded-full min-w-[32px] justify-center',
            default: {
              activeButton: {
                variant: 'outline',
              },
            },
          }"
        />
      </div>
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
import PluginCard, {
  type MarketplacePlugin,
} from "~/components/plugins/PluginCard.vue";
import InstallDialog from "~/components/plugins/InstallDialog.vue";
import { useUserStore } from "~/stores/user";

definePageMeta({
  layout: "default",
});

const q = ref("");
const category = ref("全部分类");
const status = ref("全部状态");
const sort = ref("默认排序");

// 分页相关
const currentPage = ref(1);
const pageSize = ref(10); // 确保是数字类型

const categoryOptions = [
  "全部分类",
  "数据",
  "AI",
  "可视化",
  "集成",
  "开发者工具",
];
const sortOptions = ["默认排序", "安装量", "更新时间", "名称"];
// 用户角色（用于权限控制)
const userStore = useUserStore();
const isRoot = computed(() => userStore.isRoot);
const isTenantAdmin = computed(() => userStore.isCurrentTenantAdmin);
const marketTabLabel = computed(() => "插件市场");
const installedTabLabel = computed(() => "已安装");
const menuRefreshToken = useState<number>("px-menu-refresh-token", () => 0);
const statusOptions = computed(() =>
  isRoot.value
    ? ["全部状态", "未安装", "已安装（未启用）", "已启用", "已停用", "异常"]
    : ["全部状态", "未订阅", "已订阅"]
);

// 后端数据（不使用本地 mock）
const all = ref<MarketplacePlugin[]>([]);

async function fetchMarketplace() {
  try {
    const { useAdminPluginsService } = await import(
      "~/composables/api/services/adminPluginsService"
    );
    const svc = useAdminPluginsService();
    const list = await svc.getMarketplace();
    if (Array.isArray(list)) {
      all.value = (list as any[]).map((p: any) => ({
        id: String(p.id || p.slug || p.name || ""),
        name: p.name || p.id || "-",
        description: p.description || "",
        version: p.version || "-",
        author: p.author || "",
        category: p.category || "",
        installs: Number(p.installs || p.downloadCount || 0),
        icon: p.icon,
        tags: Array.isArray(p.tags) ? p.tags : [],
        __sys: {
          isSystemInstalled: !!p.isSystemInstalled,
          isSystemEnabled: !!p.isSystemEnabled,
          systemStatus: p.systemStatus || "",
        },
        __tenant: {
          enabled: !!(p.tenantInstance?.enabled ?? p.tenantEnabled),
          status:
            p.tenantStatus ||
            p.tenantInstance?.status ||
            (p.tenantInstance?.enabled ? "enabled" : "not_enabled"),
          instance: p.tenantInstance || null,
        },
      }));
    }
  } catch (e) {
    console.error("加载市场数据失败:", e);
  }
}

onMounted(fetchMarketplace);

// 过滤 & 排序
const filtered = computed(() => {
  const list = all.value.filter((p) => {
    const ql = q.value.toLowerCase();
    const hitQ =
      !q.value ||
      p.name.toLowerCase().includes(ql) ||
      p.description.toLowerCase().includes(ql) ||
      p.author.toLowerCase().includes(ql);
    const hitC = category.value === "全部分类" || p.category === category.value;
    const sys = (p as any).__sys || {};
    const tenant = (p as any).__tenant || {};
    if (!isRoot.value && !sys.isSystemEnabled) {
      return false;
    }
    const s = String(status.value);
    let hitS = true;
    if (s !== "全部状态") {
      const isInstalled = !!sys.isSystemInstalled;
      const isEnabled = !!sys.isSystemEnabled;
      const tenantEnabled = !!tenant.enabled;
      if (isRoot.value) {
        if (s === "未安装") hitS = !isInstalled;
        else if (s === "已安装（未启用）") hitS = isInstalled && !isEnabled;
        else if (s === "已启用") hitS = isEnabled;
        else if (s === "已停用")
          hitS = isInstalled && sys.systemStatus === "disabled";
        else if (s === "异常")
          hitS = isInstalled && sys.systemStatus === "broken";
      } else {
        if (s === "已订阅") hitS = tenantEnabled;
        else if (s === "未订阅") hitS = !tenantEnabled;
      }
    }
    return hitQ && hitC && hitS;
  });
  switch (sort.value) {
    case "安装量":
      return [...list].sort((a, b) => b.installs - a.installs);
    case "名称":
      return [...list].sort((a, b) => a.name.localeCompare(b.name));
    default:
      return list;
  }
});

// 总页数（至少为 1）
const totalPages = computed(() =>
  Math.max(1, Math.ceil(filtered.value.length / pageSize.value))
);

// 分页区间（调试也用得到）
const pageStart = computed(() => (currentPage.value - 1) * pageSize.value);
const pageEnd = computed(() =>
  Math.min(pageStart.value + pageSize.value, filtered.value.length)
);

// 当前页数据（只会返回本页 0~pageSize 条）
const paginatedData = computed(() =>
  filtered.value.slice(pageStart.value, pageEnd.value)
);

// ① 搜索/分类/排序变化：回到第 1 页
watch([q, category, status, sort], () => {
  currentPage.value = 1;
});

// ② 数据源变化或 pageSize 变化：纠正越界页
watch([() => filtered.value.length, pageSize], () => {
  if (currentPage.value > totalPages.value) {
    currentPage.value = totalPages.value;
  }
});

// ③（可选）在路由切换或刷新后，若页码非法，也纠正
onMounted(() => {
  if (currentPage.value < 1) currentPage.value = 1;
  if (currentPage.value > totalPages.value)
    currentPage.value = totalPages.value;
});

function refresh() {
  fetchMarketplace();
}

const installOpen = ref(false);
const selectedPlugin = ref<MarketplacePlugin | undefined>(undefined);
function openInstall(p: MarketplacePlugin) {
  selectedPlugin.value = p;
  installOpen.value = true;
  console.info("[openInstall] fired with", p); // 便于确认点击链路没问题
}
function openInstallGeneric() {
  selectedPlugin.value = undefined as any;
  installOpen.value = true;
}
async function onInstalled(payload?: {
  plugin: MarketplacePlugin | null;
  state: any;
}) {
  try {
    // 1) 先关弹窗
    installOpen.value = false;

    // 2) 如果知道是哪个插件被安装了，先就地把那条打上“已安装/已启用”标记，避免闪烁
    const pid = payload?.plugin?.id;
    if (pid) {
      const idx = all.value.findIndex((p) => p.id === pid);
      if (idx !== -1) {
        const sys =
          (all.value[idx] as any).__sys || ((all.value[idx] as any).__sys = {});
        sys.isSystemInstalled = true;
        // 是否已启用取决于对话框勾选；拿不到就先不写死，交给刷新覆盖
        if (payload?.state?.enableAfterInstall === true) {
          sys.isSystemEnabled = true;
          sys.systemStatus = "enabled";
        }
      }
    }

    // 3) 再从后端拉一次最新市场数据，确保状态一致
    await fetchMarketplace();

    // 4) （可选）若当前过滤条件把“已安装”过滤掉了，可以考虑把 status 回到“全部状态”
    // if (status.value !== "全部状态") status.value = "全部状态"

    // 5) 保持原有选择/页码体验：如果分页越界再纠正（已有 watch，会自动处理）
  } catch (e) {
    console.error("[market.onInstalled] refresh failed:", e);
  } finally {
    // 6) 清空选中项（下次打开是“通用安装”还是从卡片打开都不受影响）
    selectedPlugin.value = undefined;
  }
}

async function toggleTenant(p: MarketplacePlugin) {
  const item = all.value.find((row) => row.id === p.id);
  if (!item) return;
  const tenant = (item as any).__tenant || ((item as any).__tenant = {});
  const enabled = !!tenant.enabled;
  const { useAdminPluginsService } = await import(
    "~/composables/api/services/adminPluginsService"
  );
  const svc = useAdminPluginsService();

  if (enabled) {
    const { useConfirm } = await import("~/composables/useConfirm");
    const { confirm } = useConfirm();
    const ok = await confirm({
      title: "停用本租户插件",
      description: "仅影响当前租户访问，不会停止平台插件进程。",
      message: `停用 ${item.name} 在本租户？`,
      confirmLabel: "停用",
      tone: "warning",
    });
    if (!ok) return;
  }

  await svc.setTenantEnabled(item.id, !enabled);
  tenant.enabled = !enabled;
  tenant.status = !enabled ? "enabled" : "disabled";
  menuRefreshToken.value += 1;
  await fetchMarketplace();
}

// 移除临时回调方案
</script>
