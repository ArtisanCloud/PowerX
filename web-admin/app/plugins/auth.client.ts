/**
 * 认证插件 - 客户端初始化
 */
export default defineNuxtPlugin(() => {
  // 初始化认证状态
  const { initAuth } = useAuth();
  initAuth();
});
