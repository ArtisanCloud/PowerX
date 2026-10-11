export default defineNuxtPlugin(async () => {
  const overlay = useOverlay();

  // 创建一个可复用的"单例" Overlay 实例
  const modal = overlay.create(
    defineAsyncComponent(() => import("~/components/GlobalLoadingModal.vue")),
    { props: { message: "加载中…" } }
  );

  // 自动加载短于 200ms 时不展示全屏遮罩；显式展示和锁屏立即生效。
  const autoVisible = useGL_AutoVisible();
  const manualVisible = useGL_ManualVisible();
  const lockCount = useGL_LockCount();
  const delayedAutoVisible = ref(false);
  watch(autoVisible, (pending, _previous, onCleanup) => {
    delayedAutoVisible.value = false;
    if (!pending) return;
    const timer = setTimeout(() => {
      delayedAutoVisible.value = true;
    }, 200);
    onCleanup(() => clearTimeout(timer));
  }, { immediate: true });
  const visible = computed(() =>
    manualVisible.value || lockCount.value > 0 || delayedAutoVisible.value
  );
  const message = useGL_Message();
  const progress = useGL_Progress();

  const opened = ref(false);

  // 文案和进度变更时，更新已打开的 modal
  watch([message, progress], ([m, p]) => {
    if (opened.value) modal.patch({ message: m, progress: p });
  });

  const openModal = async () => {
    if (opened.value || !visible.value) return;
    const instance = modal.open({
      message: message.value,
      progress: progress.value,
    });
    opened.value = true;
    await instance.result;
    opened.value = false;
    // 关闭完成前开始了另一项加载时，恢复仍然需要显示的遮罩。
    if (visible.value) void openModal();
  };
  watch(visible, (v) => {
    if (v) void openModal();
    else if (opened.value) modal.close();
  }, { immediate: true });
});
