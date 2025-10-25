export default defineNuxtPlugin(async () => {
  const overlay = useOverlay();

  // 创建一个可复用的"单例" Overlay 实例
  const modal = overlay.create(
    defineAsyncComponent(() => import("~/components/GlobalLoadingModal.vue")),
    { props: { message: "加载中…" } }
  );

  // 绑定到你已有的全局状态
  const { visible } = useGlobalLoading();
  const message = useGL_Message();
  const progress = useGL_Progress();

  const opened = ref(false);

  // 文案和进度变更时，更新已打开的 modal
  watch([message, progress], ([m, p]) => {
    if (opened.value) modal.patch({ message: m, progress: p });
  });

  // 根据 visible 打开/关闭
  watch(
    visible,
    async (v) => {
      if (v && !opened.value) {
        const instance = modal.open({
          message: message.value,
          progress: progress.value,
        });
        opened.value = true;
        // 等待关闭（无论手动 gl.hide() 还是代码 modal.close()）
        await instance.result;
        opened.value = false;
      } else if (!v && opened.value) {
        modal.close();
      }
    },
    { immediate: true }
  );
});
