export default defineNuxtPlugin((nuxtApp) => {
  const { initAuth, getToken, clearAuth } = useAuth();
  const userStore = useUserStore();
  const route = useRoute();
  const runtimeConfig = useRuntimeConfig();
  const apiBase = String(runtimeConfig.public?.apiBase || "/api").replace(/\/+$/, "");
  const setupStatusPath = `${apiBase}/admin/setup/status`;

  // 避免重复注册
  const inited = useState<boolean>("auth.__booted", () => false);
  if (inited.value) return;
  inited.value = true;

  const extractStatusCode = (error: any): number => {
    const direct = Number(error?.status || error?.statusCode || error?.response?.status || 0);
    if (direct > 0) return direct;
    const cause = error?.cause;
    return Number(cause?.status || cause?.statusCode || cause?.response?.status || 0);
  };

  const loadSetupStatus = async (): Promise<{ configured: boolean; requires_login: boolean; restart_required: boolean } | null> => {
    try {
      const resp: any = await $fetch(setupStatusPath, {
        method: "GET",
        timeout: 5000,
      });
      const payload = resp?.data ?? resp;
      return {
        configured: Boolean(payload?.configured),
        requires_login: Boolean(payload?.requires_login),
        restart_required: Boolean(payload?.restart_required),
      };
    } catch {
      return null;
    }
  };

  // 应用挂载后初始化（一定有浏览器/应用上下文，不依赖组件实例）
  nuxtApp.hook("app:mounted", async () => {
    initAuth();
    if (route.path === "/setup" || route.path.endsWith("/setup")) return;

    const setup = await loadSetupStatus();
    const shouldStayInSetup = Boolean(
      setup &&
      ((!setup.configured && !setup.requires_login) || setup.restart_required),
    );
    if (shouldStayInSetup) {
      return;
    }

    // 关键：每次刷新都强制走一次 me/context，失效 token 立即清理并跳登录
    const token = getToken();
    if (!token) return;
    try {
      await userStore.fetchUserContext({ force: true });
    } catch (error: any) {
      const statusCode = extractStatusCode(error);
      if (statusCode === 401 || statusCode === 403) {
        clearAuth();
        userStore.clearUserState();
        await navigateTo("/users/login");
      }
    }
  });
});
