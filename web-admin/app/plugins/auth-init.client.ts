export default defineNuxtPlugin((nuxtApp) => {
  const { initAuth, getToken, clearAuth } = useAuth();
  const userStore = useUserStore();

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

  // 应用挂载后初始化（一定有浏览器/应用上下文，不依赖组件实例）
  nuxtApp.hook("app:mounted", async () => {
    initAuth();

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
