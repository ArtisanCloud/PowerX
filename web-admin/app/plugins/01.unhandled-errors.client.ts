export default defineNuxtPlugin(() => {
  const IGNORED_MESSAGES = [
    "Could not establish connection. Receiving end does not exist.",
    "The message port closed before a response was received.",
    "A listener indicated an asynchronous response by returning true, but the message channel closed before a response was received.",
    "Extension context invalidated.",
  ];

  const getMessage = (reason: unknown) => {
    if (!reason) return "";
    if (typeof reason === "string") return reason;
    if (reason instanceof Error) return reason.message || "";
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const maybeMsg = (reason as any)?.message;
    return typeof maybeMsg === "string" ? maybeMsg : "";
  };

  const shouldIgnore = (reason: unknown) => {
    const msg = getMessage(reason);
    if (!msg) return false;
    return IGNORED_MESSAGES.some((m) => msg.includes(m));
  };

  // 一些浏览器扩展会向页面注入脚本并通过 message channel 通信；
  // 当页面刷新/路由切换/receiver 不存在时会抛出以上错误。
  // 这些错误与 PowerX 本身无关，但可能会触发 Nuxt 全局错误处理导致“卡死/黑屏/Loading 不结束”。
  window.addEventListener("unhandledrejection", (event) => {
    if (!shouldIgnore(event.reason)) return;
    event.preventDefault();
    console.debug("[unhandledrejection][ignored]", getMessage(event.reason));
  });

  window.addEventListener("error", (event) => {
    if (!shouldIgnore(event.error ?? event.message)) return;
    event.preventDefault();
    console.debug(
      "[window.error][ignored]",
      getMessage(event.error ?? event.message)
    );
  });
});

