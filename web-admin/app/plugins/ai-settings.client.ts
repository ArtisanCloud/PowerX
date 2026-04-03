// app/plugins/ai-settings.client.ts
import type { RouteLocationNormalized } from "vue-router";

export default defineNuxtPlugin((nuxtApp) => {
  if (!import.meta.client) return; // 只在客户端使用路由

  const initialized = useState("ai-settings.__init", () => false);
  const router = useRouter(); // ✅ 有完整 Router 类型
  const route = useRoute();

  const run = async () => {
    if (initialized.value) return;
    if (route.path === "/setup" || route.path.endsWith("/setup")) return;

    try {
      const setupResp: any = await $fetch("/api/v1/admin/setup/status", {
        method: "GET",
        timeout: 5000,
      });
      const setupPayload = setupResp?.data ?? setupResp;
      if (
        setupPayload &&
        !Boolean(setupPayload?.configured) &&
        !Boolean(setupPayload?.requires_login)
      ) {
        return;
      }
    } catch {
      // setup 状态检查失败时，不阻塞既有行为
    }

    const token = useCookie("px_token").value;
    if (!token || route.meta.auth === false) return; // 登录就绪 + 受保护页才初始化

    try {
      const userStore = useUserStore();
      if (!userStore.context) {
        await userStore.fetchUserContext();
      }
      const store = useAISettingsStore();
      await store.initialize();
      initialized.value = true;
      console.log("✅ AI Settings 初始化完成");
    } catch (err) {
      console.error("❌ AI Settings 初始化失败", err);
    }
  };

  nuxtApp.hook("app:mounted", run);
  router.afterEach((_to: RouteLocationNormalized) => run());
});
