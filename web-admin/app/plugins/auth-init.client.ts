export default defineNuxtPlugin((nuxtApp) => {
  const { initAuth } = useAuth();

  // 避免重复注册
  const inited = useState<boolean>("auth.__booted", () => false);
  if (inited.value) return;
  inited.value = true;

  // 应用挂载后初始化（一定有浏览器/应用上下文，不依赖组件实例）
  nuxtApp.hook("app:mounted", () => {
    initAuth();
  });
});
