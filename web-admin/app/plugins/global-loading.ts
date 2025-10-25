export default defineNuxtPlugin((nuxtApp) => {
  const navPending = useGL_NavPending();
  const reqPending = useGL_ReqPending();
  const autoVisible = useGL_AutoVisible();

  const recalc = () => {
    autoVisible.value = navPending.value || reqPending.value > 0;
  };

  // 路由开始/结束
  nuxtApp.hook("page:start", () => {
    navPending.value = true;
    recalc();
  });
  nuxtApp.hook("page:finish", () => {
    navPending.value = false;
    recalc();
  });

  // 监听 useFetch/asyncData 等
  nuxtApp.hook("app:fetch:before", () => {
    reqPending.value++;
    recalc();
  });
  nuxtApp.hook("app:fetch:after", () => {
    reqPending.value = Math.max(0, reqPending.value - 1);
    recalc();
  });
  nuxtApp.hook("app:fetch:error", () => {
    reqPending.value = Math.max(0, reqPending.value - 1);
    recalc();
  });

  // 包装 $fetch（覆盖默认实例，捕捉手写 $fetch 请求）
  const wrapped = $fetch.create({
    onRequest() {
      reqPending.value++;
      recalc();
    },
    onResponse() {
      reqPending.value = Math.max(0, reqPending.value - 1);
      recalc();
    },
    onRequestError() {
      reqPending.value = Math.max(0, reqPending.value - 1);
      recalc();
    },
    onResponseError() {
      reqPending.value = Math.max(0, reqPending.value - 1);
      recalc();
    },
  });

  // 暴露给 Nuxt 与全局
  // @ts-ignore
  nuxtApp.$fetch = wrapped;
  // @ts-ignore
  globalThis.$fetch = wrapped;
});
